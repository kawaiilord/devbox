package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestOwnerControlBroadcastsAuthoritativeSnapshot(t *testing.T) {
	server := NewServer(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"*"}})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	owner, _ := server.store.CreateSession("Owner")
	room, _ := server.store.CreateRoom(owner.User, "Socket room", "https://example.com/movie.mp4", 4)
	ticket, _ := server.store.IssueSocketTicket(room.Code, owner.User)
	wsURL := strings.Replace(httpServer.URL, "http://", "ws://", 1) +
		"/ws/v1/rooms/" + room.Code + "?ticket=" + ticket

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()

	_, initialMessage, err := connection.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var initial Envelope
	if err = json.Unmarshal(initialMessage, &initial); err != nil || initial.Type != "room.state" {
		t.Fatalf("initial envelope=%+v error=%v", initial, err)
	}

	payload, _ := json.Marshal(Control{Action: "play"})
	control, _ := json.Marshal(Envelope{Type: "playback.control", Seq: 1, Payload: payload})
	if err = connection.Write(ctx, websocket.MessageText, control); err != nil {
		t.Fatal(err)
	}
	_, snapshotMessage, err := connection.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot Envelope
	if err = json.Unmarshal(snapshotMessage, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Type != "playback.snapshot" {
		t.Fatalf("message type = %q, want playback.snapshot", snapshot.Type)
	}
	var playback Playback
	if err = json.Unmarshal(snapshot.Payload, &playback); err != nil {
		t.Fatal(err)
	}
	if !playback.Playing {
		t.Fatal("authoritative snapshot did not apply owner play control")
	}
}
