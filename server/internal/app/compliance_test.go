package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestComplianceMemoryDeletionAndComplaintWorkflow(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	user := AccountRecord{User: User{ID: "delete-user", Email: "delete@test", DisplayName: "Delete"}, CreatedAt: time.Now()}
	admin := AccountRecord{User: User{ID: "delete-admin", Email: "admin@test", DisplayName: "Admin", IsAdmin: true, AdminRole: "super_admin", EmailVerified: true}, CreatedAt: time.Now()}
	_ = repo.CreateUser(ctx, user)
	_ = repo.CreateUser(ctx, admin)
	request, err := repo.CreateAccountDeletionRequest(ctx, user.ID, "No longer using the service")
	if err != nil || request.Status != "pending" {
		t.Fatalf("request=%+v error=%v", request, err)
	}
	request, err = repo.ResolveAccountDeletionRequest(ctx, request.ID, admin.ID, true, "identity verified")
	if err != nil || request.Status != "approved" || request.ExecutedAt == 0 {
		t.Fatalf("resolved=%+v error=%v", request, err)
	}
	if _, err := repo.UserByID(ctx, user.ID); !errors.Is(err, ErrInvalidCredentials) && !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted user error=%v", err)
	}
	complaint, err := repo.CreateCopyrightComplaint(ctx, CopyrightComplaint{ClaimantName: "Rights Holder", ClaimantEmail: "rights@example.test", RightsBasis: "I own the exclusive distribution rights.", InfringementURL: "https://example.test/work", Evidence: []string{"https://example.test/evidence"}, StatementAccurate: true, SignatureName: "Rights Holder"})
	if err != nil || complaint.DueAt <= complaint.SubmittedAt {
		t.Fatalf("complaint=%+v error=%v", complaint, err)
	}
	complaint, err = repo.ResolveCopyrightComplaint(ctx, complaint.ID, admin.ID, "actioned", "removed")
	if err != nil || complaint.Status != "actioned" {
		t.Fatalf("resolved complaint=%+v error=%v", complaint, err)
	}
}

func TestPostgresDeletionCascadesAndRetainsOrders(t *testing.T) {
	databaseURL := testDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repo, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.pool.Exec(ctx, "TRUNCATE users CASCADE"); err != nil {
		t.Fatal(err)
	}
	tokens, _ := NewTokenManager("postgres-compliance-secret-32-characters", "postgres-compliance")
	auth := NewAuthService(repo, tokens)
	adminDevice := DeviceInfo{ID: "compliance-admin-device", Label: "Admin", Platform: "test"}
	userDevice := DeviceInfo{ID: "compliance-user-device", Label: "User", Platform: "test"}
	admin, err := auth.Register(ctx, "compliance-admin@test", "Admin", "correct horse battery", adminDevice)
	if err != nil {
		t.Fatal(err)
	}
	user, err := auth.Register(ctx, "compliance-user@test", "User", "another strong password", userDevice)
	if err != nil {
		t.Fatal(err)
	}
	_ = repo.SetUserAdmin(ctx, admin.User.ID, true)
	plan, err := repo.GetVIPPlan(ctx, "monthly")
	if err != nil {
		t.Fatal(err)
	}
	order, err := repo.CreateOrder(ctx, Order{OrderNo: "DELETE-ORDER-1", UserID: user.User.ID, PlanID: plan.ID, PlanTitle: plan.Title, AmountMinor: plan.PriceMinor, Currency: "CNY", Status: "pending", QRExpiresAt: time.Now().Add(time.Minute).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	room, err := NewStore().CreateRoom(user.User, "Delete room", "https://example.com/movie.mp4", 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	request, err := repo.CreateAccountDeletionRequest(ctx, user.User.ID, "privacy request")
	if err != nil {
		t.Fatal(err)
	}
	request, err = repo.ResolveAccountDeletionRequest(ctx, request.ID, admin.User.ID, true, "verified")
	if err != nil || request.ExecutedAt == 0 {
		t.Fatalf("request=%+v error=%v", request, err)
	}
	var userID, deletedRef string
	if err := repo.pool.QueryRow(ctx, `SELECT COALESCE(user_id,''),deleted_user_ref FROM payment_orders WHERE order_no=$1`, order.OrderNo).Scan(&userID, &deletedRef); err != nil || userID != "" || deletedRef == "" {
		t.Fatalf("retained order user=%q ref=%q error=%v", userID, deletedRef, err)
	}
	var roomCount int
	if err := repo.pool.QueryRow(ctx, `SELECT count(*) FROM rooms WHERE owner_id=$1`, user.User.ID).Scan(&roomCount); err != nil || roomCount != 0 {
		t.Fatalf("room count=%d error=%v", roomCount, err)
	}
	if _, err := repo.UserByID(ctx, user.User.ID); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("deleted user error=%v", err)
	}
}
