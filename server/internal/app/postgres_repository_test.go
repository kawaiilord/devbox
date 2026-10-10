package app

import (
	"context"
	"encoding/base64"
	"os"
	"slices"
	"strings"
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
	review, removed, err := repository.UpsertReview(ctx, Review{
		UserID: owner.User.ID, TargetType: "movie", TargetID: "tmdb:42",
		Title: "Persistent review", Rating: 9, Content: "Survives a repository reopen",
		ImageKeys: []string{"reviews/" + owner.User.ID + "/first.jpg"},
	})
	if err != nil || review.ID == 0 || len(removed) != 0 {
		t.Fatalf("review=%+v removed=%v error=%v", review, removed, err)
	}
	review, removed, err = repository.UpsertReview(ctx, Review{
		UserID: owner.User.ID, TargetType: "movie", TargetID: "tmdb:42",
		Title: "Persistent review", Rating: 10, Content: "Updated review",
	})
	if err != nil || len(removed) != 1 {
		t.Fatalf("updated review=%+v removed=%v error=%v", review, removed, err)
	}
	comment, err := repository.AddReviewComment(ctx, ReviewComment{
		ReviewID: review.ID, UserID: member.User.ID, Body: "Persistent comment",
	})
	if err != nil || comment.ID == 0 {
		t.Fatalf("review comment=%+v error=%v", comment, err)
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
	reviews, err := reopened.ListReviews(ctx, member.User.ID, "movie", "tmdb:42", 0, 10)
	if err != nil || len(reviews) != 1 || reviews[0].Rating != 10 {
		t.Fatalf("restored reviews=%+v error=%v", reviews, err)
	}
	comments, err := reopened.ListReviewComments(ctx, owner.User.ID, review.ID, 0, 10)
	if err != nil || len(comments) != 1 || comments[0].Body != "Persistent comment" {
		t.Fatalf("restored review comments=%+v error=%v", comments, err)
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
	if _, err := repository.pool.Exec(ctx, "TRUNCATE admin_audit_logs, users CASCADE"); err != nil {
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

func TestPostgresPersonalLibraryPersistenceAndSourceDeletion(t *testing.T) {
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
	tokens, _ := NewTokenManager("postgres-library-secret-with-32-characters", "postgres-library-test")
	auth := NewAuthService(repository, tokens)
	user, err := auth.Register(ctx, "library@example.test", "Library User", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	source := MediaSource{
		ID: "library-source", UserID: user.User.ID, Type: "webdav", Name: "Library",
		BaseURL: "https://example.com/dav/", CredentialsCiphertext: "encrypted",
		CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli(),
	}
	if err := repository.CreateMediaSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	favorite, err := repository.UpsertFavorite(ctx, Favorite{
		UserID: user.User.ID, SourceID: source.ID, MediaPath: "/movie.mp4",
		Title: "Movie", ContentType: "video/mp4", Size: 1024,
	})
	if err != nil || favorite.ID == 0 || favorite.SourceType != "webdav" {
		t.Fatalf("favorite=%+v error=%v", favorite, err)
	}
	record, err := repository.UpsertWatchRecord(ctx, WatchRecord{
		UserID: user.User.ID, MediaKey: strings.Repeat("a", 64),
		SourceID: source.ID, MediaPath: "/movie.mp4", Title: "Movie",
		Position: 100, Duration: 100, Completed: true, CompanionCount: 2, RoomCode: "ABC123",
	})
	if err != nil || record.ID == 0 || !record.Resumable {
		t.Fatalf("watch record=%+v error=%v", record, err)
	}
	replayed, err := repository.UpsertWatchRecord(ctx, WatchRecord{
		UserID: user.User.ID, MediaKey: strings.Repeat("a", 64),
		SourceID: source.ID, MediaPath: "/movie.mp4", Title: "Movie",
		Position: 10, Duration: 100, CompanionCount: 0, RoomCode: "ABC123",
	})
	if err != nil || replayed.ID != record.ID || !replayed.Completed || replayed.CompanionCount != 2 {
		t.Fatalf("replayed watch record=%+v error=%v", replayed, err)
	}
	favorites, err := repository.ListFavorites(ctx, user.User.ID, 0, 50)
	if err != nil || len(favorites) != 1 || favorites[0].ID != favorite.ID {
		t.Fatalf("favorites=%+v error=%v", favorites, err)
	}
	records, err := repository.ListWatchRecords(ctx, user.User.ID, 0, 50)
	if err != nil || len(records) != 1 || !records[0].Resumable {
		t.Fatalf("records=%+v error=%v", records, err)
	}
	if err := repository.DeleteMediaSource(ctx, user.User.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	favorites, err = repository.ListFavorites(ctx, user.User.ID, 0, 50)
	if err != nil || len(favorites) != 0 {
		t.Fatalf("favorites after source deletion=%+v error=%v", favorites, err)
	}
	records, err = repository.ListWatchRecords(ctx, user.User.ID, 0, 50)
	if err != nil || len(records) != 1 || records[0].Resumable || records[0].SourceID != "" {
		t.Fatalf("history after source deletion=%+v error=%v", records, err)
	}
	if err := repository.DeleteWatchRecord(ctx, user.User.ID, record.ID); err != nil {
		t.Fatal(err)
	}
	danmaku, err := repository.AddDanmaku(ctx, DanmakuMessage{
		Fingerprint: strings.Repeat("d", 64), UserID: user.User.ID,
		DisplayName: user.User.DisplayName, Body: "persistent danmaku",
		Position: 12.5, Color: 0xffffff, Mode: "scroll",
	})
	if err != nil || danmaku.ID == 0 {
		t.Fatalf("danmaku=%+v error=%v", danmaku, err)
	}
	danmakuPage, err := repository.ListDanmaku(ctx, strings.Repeat("d", 64), user.User.ID, 10, 20, 50)
	if err != nil || len(danmakuPage) != 1 || danmakuPage[0].ID != danmaku.ID {
		t.Fatalf("danmaku page=%+v error=%v", danmakuPage, err)
	}
}

func TestPostgresSocialMessagingPersistence(t *testing.T) {
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
	tokens, _ := NewTokenManager("postgres-social-secret-with-32-characters", "postgres-social-test")
	auth := NewAuthService(repository, tokens)
	alice, err := auth.Register(ctx, "pg-alice@social.test", "Alice", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := auth.Register(ctx, "pg-bob@social.test", "Bob", "another strong password")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.FollowUser(ctx, alice.User.ID, bob.User.ID); err != nil {
		t.Fatal(err)
	}
	profiles, err := repository.SearchSocialProfiles(ctx, alice.User.ID, "Bob", 10)
	if err != nil || len(profiles) != 1 || !profiles[0].Following {
		t.Fatalf("profiles=%+v error=%v", profiles, err)
	}
	conversation, err := repository.CreateConversation(ctx, alice.User.ID, bob.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	message, recipient, err := repository.AddDirectMessage(ctx, alice.User.ID, conversation.ID, "persistent hello")
	if err != nil || recipient != bob.User.ID {
		t.Fatalf("message=%+v recipient=%s error=%v", message, recipient, err)
	}
	unread, err := repository.UnreadDirectCount(ctx, bob.User.ID)
	if err != nil || unread != 1 {
		t.Fatalf("unread=%d error=%v", unread, err)
	}
	messages, err := repository.ListDirectMessages(ctx, bob.User.ID, conversation.ID, 0, 50)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages=%+v error=%v", messages, err)
	}
	if err := repository.MarkConversationRead(ctx, bob.User.ID, conversation.ID, message.ID); err != nil {
		t.Fatal(err)
	}
	unread, err = repository.UnreadDirectCount(ctx, bob.User.ID)
	if err != nil || unread != 0 {
		t.Fatalf("read unread=%d error=%v", unread, err)
	}
}

func TestPostgresCoupleSpacePersistence(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
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
	tokens, _ := NewTokenManager("postgres-couple-secret-with-32-characters", "pg-couple")
	auth := NewAuthService(repo, tokens)
	a, err := auth.Register(ctx, "a@pg-couple.test", "Alice", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	b, err := auth.Register(ctx, "b@pg-couple.test", "Bob", "another strong password")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetUserVIP(ctx, a.User.ID, time.Now().Add(24*time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	account, err := repo.UserByID(ctx, a.User.ID)
	if err != nil || account.VIPExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("vip=%+v error=%v", account, err)
	}
	request, err := repo.CreateCoupleRequest(ctx, a.User.ID, b.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	couple, err := repo.RespondCoupleRequest(ctx, b.User.ID, request.ID, true)
	if err != nil || couple.Status != "active" {
		t.Fatalf("couple=%+v error=%v", couple, err)
	}
	moment, err := repo.AddCoupleMoment(ctx, a.User.ID, "persistent moment")
	if err != nil || moment.ID == 0 {
		t.Fatalf("moment=%+v error=%v", moment, err)
	}
	couple, err = repo.SeparateCouple(ctx, a.User.ID)
	if err != nil || couple.Status != "separated" {
		t.Fatalf("separate=%+v error=%v", couple, err)
	}
	couple, err = repo.RestoreCouple(ctx, b.User.ID)
	if err != nil || couple.Status != "active" {
		t.Fatalf("restore=%+v error=%v", couple, err)
	}
}
