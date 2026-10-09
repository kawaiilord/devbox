package app

import (
	"context"
	"encoding/base64"
	"os"
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
	sources, err := reopened.ListMediaSources(ctx, owner.User.ID)
	if err != nil || len(sources) != 1 || sources[0].CredentialsCiphertext != encrypted {
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
