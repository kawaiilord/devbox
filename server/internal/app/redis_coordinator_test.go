package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestRedisCoordinatesPlaybackTicketsPresenceAndEvents(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	nodeA, err := OpenRedis(ctx, redisURL, "node-a")
	if err != nil {
		t.Fatal(err)
	}
	defer nodeA.Close()
	nodeB, err := OpenRedis(ctx, redisURL, "node-b")
	if err != nil {
		t.Fatal(err)
	}
	defer nodeB.Close()
	if err := nodeA.FlushTestNamespace(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	room := Room{
		Code: "ABC123", Name: "Redis room", OwnerID: "owner", SourceURL: "https://example.com/movie.mp4",
		MaxMembers: 8, CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Hour).UnixMilli(),
		Playback: Playback{Speed: 1, PositionTS: now.UnixMilli(), SourceVersion: 1},
	}
	if err := nodeA.InitializeRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	playback, sequence, err := nodeA.ApplyControl(ctx, room.Code, room.OwnerID, 1, Control{Action: "play"})
	if err != nil || !playback.Playing || sequence != 1 {
		t.Fatalf("playback=%+v sequence=%d error=%v", playback, sequence, err)
	}
	if _, _, err := nodeB.ApplyControl(ctx, room.Code, room.OwnerID, 1, Control{Action: "pause"}); !errors.Is(err, ErrStaleControl) {
		t.Fatalf("cross-node replay error=%v", err)
	}
	if _, _, acquired, err := nodeA.Snapshot(ctx, room.Code); err != nil || !acquired {
		t.Fatalf("node A snapshot acquired=%v error=%v", acquired, err)
	}
	if _, _, acquired, err := nodeB.Snapshot(ctx, room.Code); err != nil || acquired {
		t.Fatalf("node B snapshot acquired=%v error=%v", acquired, err)
	}

	owner := User{ID: "owner", DisplayName: "Owner"}
	ticket, err := nodeA.IssueTicket(ctx, room.Code, owner)
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := nodeB.ConsumeTicket(ctx, ticket, room.Code)
	if err != nil || consumed.ID != owner.ID {
		t.Fatalf("cross-node ticket user=%+v error=%v", consumed, err)
	}
	if _, err := nodeA.ConsumeTicket(ctx, ticket, room.Code); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("replayed ticket error=%v", err)
	}

	member := User{ID: "member", DisplayName: "Member"}
	if err := nodeA.TouchPresence(ctx, room.Code, "owner-a", owner); err != nil {
		t.Fatal(err)
	}
	if err := nodeB.TouchPresence(ctx, room.Code, "owner-b", owner); err != nil {
		t.Fatal(err)
	}
	if err := nodeB.TouchPresence(ctx, room.Code, "member-b", member); err != nil {
		t.Fatal(err)
	}
	presence, err := nodeA.Presence(ctx, room.Code)
	if err != nil || presence.OnlineCount != 2 {
		t.Fatalf("presence=%+v error=%v", presence, err)
	}

	pubsub, err := nodeB.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pubsub.Close()
	payload, _ := json.Marshal(playback)
	want := Envelope{
		Type: "playback.snapshot", Room: room.Code, Seq: sequence,
		TS: time.Now().UnixMilli(), From: owner.ID, Payload: payload,
	}
	if err := nodeA.Publish(ctx, want); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-pubsub.Channel():
		var got Envelope
		if err := json.Unmarshal([]byte(message.Payload), &got); err != nil {
			t.Fatal(err)
		}
		if got.Type != want.Type || got.Room != want.Room || got.Seq != want.Seq {
			t.Fatalf("event=%+v, want=%+v", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for cross-node event")
	}
}

func TestRedisBroadcastsPlaybackAcrossServerNodes(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	nodeA, err := OpenRedis(ctx, redisURL, "server-a")
	if err != nil {
		t.Fatal(err)
	}
	defer nodeA.Close()
	nodeB, err := OpenRedis(ctx, redisURL, "server-b")
	if err != nil {
		t.Fatal(err)
	}
	defer nodeB.Close()
	if err := nodeA.FlushTestNamespace(ctx); err != nil {
		t.Fatal(err)
	}

	repository := NewMemoryRepository()
	tokens, _ := NewTokenManager("redis-server-test-secret-with-32-characters", "sameframe-redis-test")
	auth := NewAuthService(repository, tokens)
	owner, err := auth.Register(ctx, "owner@redis.test", "Owner", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	member, err := auth.Register(ctx, "member@redis.test", "Member", "another strong password")
	if err != nil {
		t.Fatal(err)
	}
	seed := NewStore()
	room, err := seed.CreateRoom(owner.User, "Two node room", "https://example.com/movie.mp4", 4)
	if err != nil {
		t.Fatal(err)
	}
	room, err = seed.JoinRoom(room.Code, member.User)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	if err := nodeA.InitializeRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	serverA := NewServer(Options{
		Logger: logger, AllowedOrigins: []string{"*"}, Repository: repository,
		Auth: auth, InitialRooms: []Room{room}, Redis: nodeA,
	})
	serverB := NewServer(Options{
		Logger: logger, AllowedOrigins: []string{"*"}, Repository: repository,
		Auth: auth, InitialRooms: []Room{room}, Redis: nodeB,
	})
	serverA.StartBackground(ctx)
	serverB.StartBackground(ctx)
	httpA := httptest.NewServer(serverA.Handler())
	defer httpA.Close()
	httpB := httptest.NewServer(serverB.Handler())
	defer httpB.Close()

	ownerTicket := issueTestSocketTicket(t, httpA.URL, room.Code, owner.AccessToken)
	memberTicket := issueTestSocketTicket(t, httpB.URL, room.Code, member.AccessToken)
	ownerSocket := dialTestRoomSocket(t, ctx, httpA.URL, room.Code, ownerTicket)
	defer ownerSocket.CloseNow()
	memberSocket := dialTestRoomSocket(t, ctx, httpB.URL, room.Code, memberTicket)
	defer memberSocket.CloseNow()
	readTestEnvelope(t, ctx, ownerSocket, "room.state")
	readTestEnvelope(t, ctx, memberSocket, "room.state")

	payload, _ := json.Marshal(Control{Action: "play"})
	control, _ := json.Marshal(Envelope{Type: "playback.control", Seq: 1, Payload: payload})
	if err := ownerSocket.Write(ctx, websocket.MessageText, control); err != nil {
		t.Fatal(err)
	}
	snapshot := readTestEnvelope(t, ctx, memberSocket, "playback.snapshot")
	var playback Playback
	if err := json.Unmarshal(snapshot.Payload, &playback); err != nil {
		t.Fatal(err)
	}
	if !playback.Playing {
		t.Fatal("node B did not receive node A playback control")
	}
}

func issueTestSocketTicket(t *testing.T, baseURL, roomCode, accessToken string) string {
	t.Helper()
	body := requestJSON(
		t,
		http.MethodPost,
		baseURL+"/api/v1/rooms/"+roomCode+"/socket-ticket",
		accessToken,
		nil,
	)
	var response struct {
		Data struct {
			Ticket string `json:"ticket"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	return response.Data.Ticket
}

func dialTestRoomSocket(
	t *testing.T,
	ctx context.Context,
	baseURL, roomCode, ticket string,
) *websocket.Conn {
	t.Helper()
	url := strings.Replace(baseURL, "http://", "ws://", 1) +
		"/ws/v1/rooms/" + roomCode + "?ticket=" + ticket
	connection, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return connection
}

func readTestEnvelope(
	t *testing.T,
	ctx context.Context,
	connection *websocket.Conn,
	wantType string,
) Envelope {
	t.Helper()
	for attempts := 0; attempts < 12; attempts++ {
		_, message, err := connection.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var envelope Envelope
		if json.Unmarshal(message, &envelope) == nil && envelope.Type == wantType {
			return envelope
		}
	}
	t.Fatalf("did not receive %s envelope", wantType)
	return Envelope{}
}
