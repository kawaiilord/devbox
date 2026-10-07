package app

import (
	"errors"
	"testing"
	"time"
)

func TestRoomOwnerControlsAndProjection(t *testing.T) {
	store := NewStore()
	clock := time.Unix(1000, 0)
	store.now = func() time.Time { return clock }
	ownerSession, _ := store.CreateSession("Owner")
	memberSession, _ := store.CreateSession("Member")
	room, err := store.CreateRoom(ownerSession.User, "Friday movie", "https://example.com/movie.mp4", 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.JoinRoom(room.Code, memberSession.User); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.ApplyControl(room.Code, memberSession.User, 1, Control{Action: "play"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member control error = %v, want forbidden", err)
	}
	updated, _, err := store.ApplyControl(room.Code, ownerSession.User, 1, Control{Action: "play"})
	if err != nil || !updated.Playback.Playing {
		t.Fatalf("owner play failed: room=%+v err=%v", updated, err)
	}
	clock = clock.Add(5 * time.Second)
	projectedRoom, _, err := store.Snapshot(room.Code)
	if err != nil {
		t.Fatal(err)
	}
	if projectedRoom.Playback.Position != 5 {
		t.Fatalf("position = %v, want 5", projectedRoom.Playback.Position)
	}
	if _, _, err = store.ApplyControl(room.Code, ownerSession.User, 1, Control{Action: "pause"}); !errors.Is(err, ErrStaleControl) {
		t.Fatalf("replayed control error = %v, want stale", err)
	}
}

func TestSourceURLRejectsEmbeddedCredentials(t *testing.T) {
	store := NewStore()
	session, _ := store.CreateSession("Owner")
	_, err := store.CreateRoom(session.User, "Secret source", "https://user:password@example.com/movie.mp4", 2)
	if err == nil {
		t.Fatal("expected embedded credentials to be rejected")
	}
}

func TestSocketTicketIsScopedAndOneTime(t *testing.T) {
	store := NewStore()
	session, _ := store.CreateSession("Owner")
	room, _ := store.CreateRoom(session.User, "Ticket room", "https://example.com/movie.mp4", 2)
	ticket, err := store.IssueSocketTicket(room.Code, session.User)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ConsumeSocketTicket(ticket, "WRONG1"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong-room ticket error = %v, want unauthorized", err)
	}
	if _, err = store.ConsumeSocketTicket(ticket, room.Code); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("reused ticket error = %v, want unauthorized", err)
	}

	secondTicket, _ := store.IssueSocketTicket(room.Code, session.User)
	user, err := store.ConsumeSocketTicket(secondTicket, room.Code)
	if err != nil || user.ID != session.User.ID {
		t.Fatalf("valid ticket user=%+v error=%v", user, err)
	}
}
