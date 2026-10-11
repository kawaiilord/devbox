package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestApprovalLifecycle(t *testing.T) { testApprovalLifecycle(t, NewMemoryRepository()) }
func TestPostgresApprovalLifecycle(t *testing.T) {
	repo, err := OpenPostgres(context.Background(), testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.pool.Exec(context.Background(), "TRUNCATE users,admin_documents CASCADE"); err != nil {
		t.Fatal(err)
	}
	defer repo.pool.Exec(context.Background(), "TRUNCATE users,admin_documents CASCADE")
	testApprovalLifecycle(t, repo)
	// Re-running the embedded migrations must preserve pending statuses.
	if err = repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func testApprovalLifecycle(t *testing.T, repo Repository) {
	f := newManagementFixture(t, repo)
	ctx := context.Background()
	policy := defaultSitePolicy()
	policy.RegistrationRequiresReview = true
	policy.RoomCreationRequiresReview = true
	policy.MaxRoomsPerUser = 1
	f.call(t, "PUT", "/api/v1/admin/site-policy", map[string]any{"version": 0, "policy": policy}, f.root.AccessToken, 200)
	_, err := f.auth.Register(ctx, "waiting@example.test", "Waiting", "strong password 123", f.device)
	var pending *ApprovalPendingError
	if !errors.As(err, &pending) || pending.RequestID == "" {
		t.Fatalf("signup: %v", err)
	}
	if _, err = f.auth.Login(ctx, "waiting@example.test", "strong password 123", f.device); !errors.As(err, &pending) {
		t.Fatalf("pending login: %v", err)
	}
	requests, _, err := repo.FindApprovals(ctx, f.root.User.ID, "admin", "", AdminQuery{Kind: "registration", Limit: 20})
	if err != nil || len(requests) != 1 {
		t.Fatalf("queue %v %v", requests, err)
	}
	signup := requests[0]
	f.call(t, "POST", "/api/v1/approvals/"+signup.ID+"/resolve", map[string]any{"version": 1, "approve": true}, f.operator.AccessToken, 403)
	f.call(t, "POST", "/api/v1/approvals/"+signup.ID+"/resolve", map[string]any{"version": 1, "approve": true}, f.root.AccessToken, 200)
	f.call(t, "POST", "/api/v1/approvals/"+signup.ID+"/resolve", map[string]any{"version": 1, "approve": true}, f.root.AccessToken, 409)
	if _, err = f.auth.Login(ctx, "waiting@example.test", "strong password 123", f.device); err != nil {
		t.Fatal(err)
	}
	f.call(t, "POST", "/api/v1/rooms", map[string]any{"name": "Review", "source_url": "https://example.com/v.mp4", "settings": map[string]any{"visibility": "public", "require_approval": true}}, f.member.AccessToken, 202)
	f.call(t, "POST", "/api/v1/rooms", map[string]any{"name": "Review2", "source_url": "https://example.com/v.mp4"}, f.member.AccessToken, 403)
	requests, _, err = repo.FindApprovals(ctx, f.member.User.ID, "mine", "", AdminQuery{Kind: "room_creation", Limit: 20})
	if err != nil || len(requests) != 1 {
		t.Fatalf("queue %v %v", requests, err)
	}
	creation := requests[0]
	public, err := repo.DiscoverRooms(ctx, RoomDiscoveryQuery{Limit: 20})
	if err != nil || len(public) != 0 {
		t.Fatalf("pending room discoverable %v %v", public, err)
	}
	f.call(t, "POST", "/api/v1/approvals/"+creation.ID+"/resolve", map[string]any{"version": 1, "approve": true}, f.root.AccessToken, 200)
	f.call(t, "GET", "/api/v1/rooms/"+creation.RoomCode, nil, f.member.AccessToken, 200)
	for i := 0; i < 2; i++ {
		f.call(t, "POST", "/api/v1/rooms/"+creation.RoomCode+"/join", nil, f.operator.AccessToken, 202)
	}
	var queue struct {
		Requests []ApprovalRequest `json:"requests"`
	}
	json.Unmarshal(f.call(t, "GET", "/api/v1/rooms/"+creation.RoomCode+"/join-requests", nil, f.member.AccessToken, 200), &queue)
	if len(queue.Requests) != 1 {
		t.Fatalf("duplicate requests %+v", queue)
	}
	join := queue.Requests[0]
	if join.Email != "" {
		t.Fatal("owner queue leaked account email")
	}
	f.call(t, "POST", "/api/v1/approvals/"+join.ID+"/resolve", map[string]any{"version": 1, "approve": true}, f.member.AccessToken, 200)
	f.call(t, "GET", "/api/v1/rooms/"+creation.RoomCode, nil, f.operator.AccessToken, 200)
	if _, _, err = repo.FindApprovals(ctx, f.member.User.ID, "unknown", "", AdminQuery{Limit: 20}); !errors.Is(err, ErrForbidden) {
		t.Fatal("unrecognized scope bypass")
	}
	if _, _, err = repo.FindApprovals(ctx, "missing", "mine", "", AdminQuery{Limit: 20}); !errors.Is(err, ErrForbidden) {
		t.Fatal("missing actor accepted")
	}
	_, err = f.auth.Register(ctx, "reject@example.test", "Rejected", "strong password 123", f.device)
	if !errors.As(err, &pending) {
		t.Fatal(err)
	}
	id := pending.RequestID
	a, _ := repo.UserByEmail(ctx, "reject@example.test")
	if _, err = repo.AdminMutateUser(ctx, a.ID, f.root.User.ID, AdminUserChange{Version: a.AdminVersion, Action: "ban", Reason: "abuse"}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.DecideApproval(ctx, id, f.root.User.ID, 1, true, ""); !errors.Is(err, ErrAdminConflict) {
		t.Fatal("approval undid ban")
	}
	if _, err = repo.DecideApproval(ctx, id, f.root.User.ID, 1, false, "closed after ban"); err != nil {
		t.Fatal(err)
	}
	a, _ = repo.UserByID(ctx, a.ID)
	if a.AccountStatus != "banned" {
		t.Fatal("rejection undid ban")
	}
	_, err = f.auth.Register(ctx, "pending@example.test", "Pending", "strong password 123", f.device)
	if !errors.As(err, &pending) {
		t.Fatal(err)
	}
}
