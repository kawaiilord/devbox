package app

import (
	"context"
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
	auth := NewAuthService(repository, tokens)
	owner, err := auth.Register(ctx, "owner@example.com", "Owner", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	member, err := auth.Register(ctx, "member@example.com", "Member", "another strong password")
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	room, err := store.CreateRoom(owner.User, "Persistent room", "https://example.com/movie.mp4", 4)
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
	freshAuth := NewAuthService(reopened, tokens)
	if _, err := freshAuth.Login(ctx, "owner@example.com", "correct horse battery"); err != nil {
		t.Fatalf("login after reopen: %v", err)
	}
}
