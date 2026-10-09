package app

import (
	"context"
	"encoding/base64"
	"os"
	"slices"
	"testing"
	"time"
)

func TestPostgresPersistsAccountsRoomsAndMembers(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(
		ctx,
		"TRUNCATE refresh_tokens, room_members, rooms, users CASCADE",
	); err != nil {
		t.Fatal(err)
	}

	tokens, _ := NewTokenManager("postgres-test-secret-with-more-than-32-characters", "sameframe-postgres-test")
	mailer := &MemoryMailer{}
	auth := NewAuthService(repository, tokens, mailer)
	owner, err := auth.Register(ctx, "owner@example.com", "Owner", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	verification, ok := mailer.LastMessage()
	if !ok || verification.Type != "verify_email" {
		t.Fatal("verification token was not persisted and queued")
	}
	if err := auth.VerifyEmail(ctx, verification.Token); err != nil {
		t.Fatal(err)
	}
	member, err := auth.Register(ctx, "member@example.com", "Member", "another strong password")
	if err != nil {
		t.Fatal(err)
	}
	vaultKey := make([]byte, 32)
	vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(vaultKey))
	encrypted, err := vault.Encrypt(
		[]byte(`{"username":"viewer","password":"secret"}`),
		sourceAAD(owner.User.ID, "source-postgres"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateMediaSource(ctx, MediaSource{
		ID: "source-postgres", UserID: owner.User.ID, Type: "webdav", Name: "Persistent source",
		BaseURL: "https://example.com/dav/", CredentialsCiphertext: encrypted,
		CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	embyEncrypted, err := vault.Encrypt(
		[]byte(`{"user_id":"emby-user","access_token":"encrypted-token","device_id":"device-id"}`),
		sourceAAD(owner.User.ID, "source-emby"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateMediaSource(ctx, MediaSource{
		ID: "source-emby", UserID: owner.User.ID, Type: "emby", Name: "Persistent Emby",
		BaseURL: "https://emby.example.com/emby/", CredentialsCiphertext: embyEncrypted,
		CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	room, err := store.CreateMediaRoom(
		owner.User, "Persistent room", "source-postgres", "/movie.mp4", 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	room, err = store.JoinRoom(room.Code, member.User)
	if err != nil {
		t.Fatal(err)
	}
	var joinedMember Member
	for _, candidate := range room.Members {
		if candidate.UserID == member.User.ID {
			joinedMember = candidate
			break
		}
	}
	if joinedMember.UserID == "" {
		t.Fatal("joined member not present in room snapshot")
	}
	if err := repository.SaveMember(ctx, room.Code, joinedMember); err != nil {
		t.Fatal(err)
	}
	position := 42.5
	room, _, err = store.ApplyControl(room.Code, owner.User, 1, Control{Action: "seek", Position: &position})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdatePlayback(ctx, room.Code, room.Playback); err != nil {
		t.Fatal(err)
	}
	chat, err := repository.AddRoomMessage(ctx, ChatMessage{
		RoomCode: room.Code, UserID: member.User.ID,
		DisplayName: member.User.DisplayName, Body: "persistent hello",
	})
	if err != nil || chat.ID == 0 {
		t.Fatalf("chat=%+v error=%v", chat, err)
	}
	repository.Close()

	reopened, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rooms, err := reopened.LoadRooms(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 1 || len(rooms[0].Members) != 2 {
		t.Fatalf("restored rooms=%+v", rooms)
	}
	if rooms[0].Playback.Position != position {
		t.Fatalf("restored position=%v, want %v", rooms[0].Playback.Position, position)
	}
	if rooms[0].MediaSourceID != "source-postgres" || rooms[0].MediaPath != "/movie.mp4" {
		t.Fatalf("restored media reference=%+v", rooms[0])
	}
	messages, err := reopened.ListRoomMessages(ctx, room.Code, owner.User.ID, 0, 50)
	if err != nil || len(messages) != 1 || messages[0].Body != "persistent hello" {
		t.Fatalf("restored messages=%+v error=%v", messages, err)
	}
	sources, err := reopened.ListMediaSources(ctx, owner.User.ID)
	if err != nil || len(sources) != 2 ||
		(sources[0].CredentialsCiphertext != encrypted && sources[1].CredentialsCiphertext != encrypted) ||
		(sources[0].CredentialsCiphertext != embyEncrypted && sources[1].CredentialsCiphertext != embyEncrypted) {
		t.Fatalf("restored sources=%+v error=%v", sources, err)
	}
	freshAuth := NewAuthService(reopened, tokens)
	login, err := freshAuth.Login(ctx, "owner@example.com", "correct horse battery")
	if err != nil {
		t.Fatalf("login after reopen: %v", err)
	}
	if !login.User.EmailVerified {
		t.Fatal("email verification did not persist")
	}
	devices, err := reopened.UserDevices(ctx, owner.User.ID)
	if err != nil || len(devices) != 1 {
		t.Fatalf("devices after reopen=%+v error=%v", devices, err)
	}
}

func TestPostgresModerationPersistenceAndAuditChain(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, "TRUNCATE users CASCADE"); err != nil {
		t.Fatal(err)
	}
	tokens, _ := NewTokenManager("postgres-moderation-secret-with-32-characters", "postgres-moderation-test")
	auth := NewAuthService(repository, tokens)
	adminDevice := DeviceInfo{ID: "postgres-admin-device", Label: "Admin", Platform: "test"}
	memberDevice := DeviceInfo{ID: "postgres-member-device", Label: "Member", Platform: "test"}
	admin, err := auth.Register(ctx, "pg-admin@example.test", "Admin", "correct horse battery", adminDevice)
	if err != nil {
		t.Fatal(err)
	}
	member, err := auth.Register(ctx, "pg-member@example.test", "Member", "another strong password", memberDevice)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SetUserAdmin(ctx, admin.User.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := repository.VerifyEmail(ctx, admin.User.ID); err != nil {
		t.Fatal(err)
	}
	account, err := repository.UserByID(ctx, admin.User.ID)
	if err != nil || !account.IsAdmin {
		t.Fatalf("admin account=%+v error=%v", account, err)
	}

	privacy := PrivacySettings{AllowProfileFind: true}
	if err := repository.UpdatePrivacy(ctx, member.User.ID, privacy); err != nil {
		t.Fatal(err)
	}
	storedPrivacy, err := repository.GetPrivacy(ctx, member.User.ID)
	if err != nil || storedPrivacy != privacy {
		t.Fatalf("privacy=%+v error=%v", storedPrivacy, err)
	}
	if err := repository.BlockUser(ctx, member.User.ID, admin.User.ID); err != nil {
		t.Fatal(err)
	}
	blocked, err := repository.UsersBlocked(ctx, admin.User.ID, member.User.ID)
	if err != nil || !blocked {
		t.Fatalf("blocked=%v error=%v", blocked, err)
	}
	blockedUsers, err := repository.ListBlockedUsers(ctx, member.User.ID)
	if err != nil || len(blockedUsers) != 1 || blockedUsers[0].ID != admin.User.ID {
		t.Fatalf("blocked users=%+v error=%v", blockedUsers, err)
	}

	store := NewStore()
	room, err := store.CreateRoom(admin.User, "Moderated room", "https://example.com/movie.mp4", 4)
	if err != nil {
		t.Fatal(err)
	}
	room, err = store.JoinRoom(room.Code, member.User)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	adminMessage, err := repository.AddRoomMessage(ctx, ChatMessage{
		RoomCode: room.Code, UserID: admin.User.ID, DisplayName: admin.User.DisplayName, Body: "hidden",
	})
	if err != nil {
		t.Fatal(err)
	}
	memberMessage, err := repository.AddRoomMessage(ctx, ChatMessage{
		RoomCode: room.Code, UserID: member.User.ID, DisplayName: member.User.DisplayName, Body: "visible",
	})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := repository.ListRoomMessages(ctx, room.Code, member.User.ID, 0, 50)
	if err != nil || len(messages) != 1 || messages[0].ID != memberMessage.ID {
		t.Fatalf("filtered messages=%+v hidden=%+v error=%v", messages, adminMessage, err)
	}

	report, err := repository.CreateReport(ctx, Report{
		ReporterID: member.User.ID, TargetType: "room", TargetID: room.Code,
		Reason: "harassment", Details: "review",
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err = repository.ResolveReport(ctx, report.ID, admin.User.ID, "actioned", "closed")
	if err != nil || report.ReviewedBy != admin.User.ID || report.Status != "actioned" {
		t.Fatalf("resolved report=%+v error=%v", report, err)
	}
	if err := repository.CloseRoom(ctx, room.Code, "missing-admin"); err == nil {
		t.Fatal("room close without a valid audit actor unexpectedly succeeded")
	}
	roomsBeforeClose, err := repository.LoadRooms(ctx)
	if err != nil || len(roomsBeforeClose) != 1 || roomsBeforeClose[0].Closed {
		t.Fatalf("room mutation was not rolled back with failed audit: rooms=%+v error=%v", roomsBeforeClose, err)
	}
	if err := repository.CloseRoom(ctx, room.Code, admin.User.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.BanDevice(ctx, hashDeviceID(memberDevice.ID), "abuse", admin.User.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.ValidateDevice(ctx, member.User.ID, hashDeviceID(memberDevice.ID)); err != ErrDeviceBlocked {
		t.Fatalf("banned device validation error=%v", err)
	}

	for _, event := range []AuditEvent{
		{ActorID: admin.User.ID, Action: "report.resolve", TargetType: "report", TargetID: strconvFormat(report.ID)},
		{ActorID: admin.User.ID, Action: "room.close", TargetType: "room", TargetID: room.Code},
	} {
		if _, err := repository.AppendAudit(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	events, err := repository.ListAudit(ctx, 0, 50)
	if err != nil || len(events) != 5 || !verifyAuditPage(events) {
		t.Fatalf("audit events=%+v error=%v", events, err)
	}
	slices.Reverse(events)
	if !verifyAuditChain(events) {
		t.Fatal("persisted audit chain is invalid")
	}
	rooms, err := repository.LoadRooms(ctx)
	if err != nil || len(rooms) != 1 || !rooms[0].Closed {
		t.Fatalf("closed rooms=%+v error=%v", rooms, err)
	}
}
