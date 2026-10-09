package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestTURNRestCredentialsAndTargetedRTCSignaling(t *testing.T) {
	manager, err := NewRTCConfigManager([]string{"turns:turn.example.com:5349?transport=tcp"}, "0123456789abcdef-secret")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	servers, expires := manager.Servers("user-a", now)
	if expires != now.Add(time.Hour).UnixMilli() {
		t.Fatalf("expires=%d", expires)
	}
	mac := hmac.New(sha1.New, []byte("0123456789abcdef-secret"))
	_, _ = mac.Write([]byte(servers[0].Username))
	if servers[0].Credential != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatal("invalid TURN credential")
	}
	repo := NewMemoryRepository()
	ctx := context.Background()
	owner := AccountRecord{User: User{ID: "owner", DisplayName: "Owner", Email: "owner@test"}}
	member := AccountRecord{User: User{ID: "member", DisplayName: "Member", Email: "member@test"}}
	other := AccountRecord{User: User{ID: "other", DisplayName: "Other", Email: "other@test"}}
	_ = repo.CreateUser(ctx, owner)
	_ = repo.CreateUser(ctx, member)
	_ = repo.CreateUser(ctx, other)
	server := NewServer(Options{Repository: repo, RTC: manager})
	room, _ := server.store.CreateRoom(owner.User, "Voice room", "https://example.com/movie.mp4", 4)
	room, _ = server.store.JoinRoom(room.Code, member.User)
	room, _ = server.store.JoinRoom(room.Code, other.User)
	targetClient := &socketClient{user: member.User, send: make(chan []byte, 1)}
	otherClient := &socketClient{user: other.User, send: make(chan []byte, 1)}
	senderClient := &socketClient{user: owner.User, send: make(chan []byte, 1)}
	defer server.hub.Subscribe(room.Code, targetClient)()
	defer server.hub.Subscribe(room.Code, otherClient)()
	payload, _ := json.Marshal(RTCSignal{Type: "offer", TargetUserID: member.ID, SDP: "v=0"})
	server.handleRTCSignal(ctx, senderClient, room.Code, owner.User, payload)
	select {
	case message := <-targetClient.send:
		var envelope Envelope
		_ = json.Unmarshal(message, &envelope)
		if envelope.Type != "rtc.signal" {
			t.Fatalf("envelope=%+v", envelope)
		}
	default:
		t.Fatal("target did not receive RTC signal")
	}
	select {
	case <-otherClient.send:
		t.Fatal("non-target received RTC signal")
	default:
	}
}
