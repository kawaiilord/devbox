package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"
)

func TestBlockedUsersAreFilteredFromHistoryAndRealtime(t *testing.T) {
	repository := NewMemoryRepository()
	ctx := context.Background()
	userA := AccountRecord{User: User{ID: "user-a", DisplayName: "Alice", Email: "alice@example.test"}}
	userB := AccountRecord{User: User{ID: "user-b", DisplayName: "Bob", Email: "bob@example.test"}}
	if err := repository.CreateUser(ctx, userA); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateUser(ctx, userB); err != nil {
		t.Fatal(err)
	}
	messageA, _ := repository.AddRoomMessage(ctx, ChatMessage{
		RoomCode: "ABC123", UserID: userA.ID, DisplayName: userA.DisplayName, Body: "from Alice",
	})
	messageB, _ := repository.AddRoomMessage(ctx, ChatMessage{
		RoomCode: "ABC123", UserID: userB.ID, DisplayName: userB.DisplayName, Body: "from Bob",
	})
	if err := repository.BlockUser(ctx, userB.ID, userA.ID); err != nil {
		t.Fatal(err)
	}
	history, err := repository.ListRoomMessages(ctx, "ABC123", userA.ID, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != messageA.ID {
		t.Fatalf("history after reverse-direction block=%+v, messageB=%+v", history, messageB)
	}

	server := NewServer(Options{Repository: repository})
	server.store.RestoreRooms([]Room{{Code: "ABC123", OwnerID: userA.ID, ExpiresAt: time.Now().Add(time.Hour).UnixMilli(), Members: []Member{{UserID: userA.ID}, {UserID: userB.ID}}}})
	clientA := &socketClient{user: userA.User, send: make(chan []byte, 1)}
	clientB := &socketClient{user: userB.User, send: make(chan []byte, 1)}
	unsubscribeA := server.hub.Subscribe("ABC123", clientA)
	defer unsubscribeA()
	unsubscribeB := server.hub.Subscribe("ABC123", clientB)
	defer unsubscribeB()
	payload, _ := json.Marshal(messageA)
	server.broadcastLocal(ctx, Envelope{
		Type: "chat.message", Room: "ABC123", From: userA.ID, Payload: payload,
	})
	select {
	case <-clientA.send:
	default:
		t.Fatal("sender did not receive own chat message")
	}
	select {
	case leaked := <-clientB.send:
		t.Fatalf("blocked chat leaked to recipient: %s", leaked)
	default:
	}
}

func TestModerationAPIDefaultDenyActionsAndAuditChain(t *testing.T) {
	repository := NewMemoryRepository()
	tokens, _ := NewTokenManager("moderation-test-secret-with-at-least-32-characters", "sameframe-moderation-test")
	auth := NewAuthService(repository, tokens)
	ctx := context.Background()
	adminDevice := DeviceInfo{ID: "moderation-admin-device", Label: "Admin", Platform: "test"}
	memberDevice := DeviceInfo{ID: "moderation-member-device", Label: "Member", Platform: "test"}
	admin, err := auth.Register(ctx, "admin@example.test", "Admin", "correct horse battery", adminDevice)
	if err != nil {
		t.Fatal(err)
	}
	member, err := auth.Register(ctx, "member@example.test", "Member", "another strong password", memberDevice)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SetUserAdmin(ctx, admin.User.ID, true); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(Options{
		Logger: logger, AllowedOrigins: []string{"*"}, Repository: repository, Auth: auth,
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	unverifiedAdminStatus := requestStatusDevice(
		t, http.MethodGet, httpServer.URL+"/api/v1/admin/audit",
		admin.AccessToken, nil, adminDevice,
	)
	if unverifiedAdminStatus != http.StatusForbidden {
		t.Fatalf("unverified admin status=%d, want 403", unverifiedAdminStatus)
	}
	if err := repository.VerifyEmail(ctx, admin.User.ID); err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{
		"/api/v1/admin/reports",
		"/api/v1/admin/audit",
		"/api/v1/admin/reports/1/resolve",
		"/api/v1/admin/rooms/ABC123/close",
		"/api/v1/admin/devices/" + hashDeviceID(adminDevice.ID) + "/ban",
	} {
		method := http.MethodGet
		if target != "/api/v1/admin/reports" && target != "/api/v1/admin/audit" {
			method = http.MethodPost
		}
		status := requestStatusDevice(t, method, httpServer.URL+target, member.AccessToken, nil, memberDevice)
		if status != http.StatusForbidden {
			t.Fatalf("non-admin %s %s status=%d, want 403", method, target, status)
		}
	}

	roomBody := requestJSONDevice(
		t, http.MethodPost, httpServer.URL+"/api/v1/rooms", admin.AccessToken,
		map[string]any{"name": "Moderated room", "source_url": "https://example.com/movie.mp4", "max_members": 4},
		adminDevice,
	)
	var created struct {
		Data Room `json:"data"`
	}
	if err := json.Unmarshal(roomBody, &created); err != nil {
		t.Fatal(err)
	}
	requestJSONDevice(
		t, http.MethodPost, httpServer.URL+"/api/v1/rooms/"+created.Data.Code+"/join",
		member.AccessToken, nil, memberDevice,
	)
	reportBody := requestJSONDevice(
		t, http.MethodPost, httpServer.URL+"/api/v1/reports", member.AccessToken,
		map[string]string{
			"target_type": "room", "target_id": created.Data.Code,
			"reason": "harassment", "details": "Please review this room",
		},
		memberDevice,
	)
	var reported struct {
		Data Report `json:"data"`
	}
	if err := json.Unmarshal(reportBody, &reported); err != nil {
		t.Fatal(err)
	}
	requestJSONDevice(
		t, http.MethodPost,
		httpServer.URL+"/api/v1/admin/reports/"+strconvFormat(reported.Data.ID)+"/resolve",
		admin.AccessToken, map[string]string{"status": "actioned", "resolution": "Reviewed"}, adminDevice,
	)
	requestJSONDevice(
		t, http.MethodPost, httpServer.URL+"/api/v1/admin/rooms/"+created.Data.Code+"/close",
		admin.AccessToken, nil, adminDevice,
	)
	joinStatus := requestStatusDevice(
		t, http.MethodPost, httpServer.URL+"/api/v1/rooms/"+created.Data.Code+"/join",
		member.AccessToken, nil, memberDevice,
	)
	if joinStatus != http.StatusForbidden {
		t.Fatalf("closed room join status=%d, want 403", joinStatus)
	}
	requestJSONDevice(
		t, http.MethodPost,
		httpServer.URL+"/api/v1/admin/devices/"+hashDeviceID(memberDevice.ID)+"/ban",
		admin.AccessToken, map[string]string{"reason": "abuse"}, adminDevice,
	)
	accessStatus := requestStatusDevice(
		t, http.MethodGet, httpServer.URL+"/api/v1/users/me", member.AccessToken, nil, memberDevice,
	)
	if accessStatus != http.StatusUnauthorized {
		t.Fatalf("banned device access status=%d, want 401", accessStatus)
	}

	events, err := repository.ListAudit(ctx, 0, 50)
	if err != nil || len(events) != 3 {
		t.Fatalf("audit events=%+v error=%v", events, err)
	}
	slices.Reverse(events)
	if !verifyAuditChain(events) {
		t.Fatal("valid audit chain was rejected")
	}
	redacted := finalizeAuditEvent(AuditEvent{Metadata: map[string]any{
		"status": "ok", "access_token": "must-not-appear", "password": "must-not-appear",
	}})
	if len(redacted.Metadata) != 1 || redacted.Metadata["status"] != "ok" {
		t.Fatalf("sensitive audit metadata was not removed: %+v", redacted.Metadata)
	}
	events[1].TargetID = "tampered"
	if verifyAuditChain(events) {
		t.Fatal("tampered audit chain was accepted")
	}
}

func strconvFormat(value int64) string { return strconv.FormatInt(value, 10) }
