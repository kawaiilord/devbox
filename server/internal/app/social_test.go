package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestSocialSearchFollowMessagingUnreadAndRealtime(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	tokens, _ := NewTokenManager("social-messaging-test-secret-at-least-32-characters", "social-test")
	auth := NewAuthService(repository, tokens)
	aliceDevice := DeviceInfo{ID: "social-alice-device", Label: "Alice", Platform: "test"}
	bobDevice := DeviceInfo{ID: "social-bob-device-01", Label: "Bob", Platform: "test"}
	alice, err := auth.Register(ctx, "alice@social.test", "Alice", "correct horse battery", aliceDevice)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := auth.Register(ctx, "bob@social.test", "Bob", "another strong password", bobDevice)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdatePrivacy(ctx, bob.User.ID, PrivacySettings{
		AllowRoomChat: true, AllowPrivateChat: true, AllowProfileFind: true, ShowWatchActivity: true,
	}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"*"}, Repository: repository, Auth: auth})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	searchBody := requestJSONDevice(t, http.MethodGet, httpServer.URL+"/api/v1/social/users?q=Bob", alice.AccessToken, nil, aliceDevice)
	var search struct {
		Data struct {
			Users []SocialProfile `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(searchBody, &search); err != nil || len(search.Data.Users) != 1 || search.Data.Users[0].ID != bob.User.ID {
		t.Fatalf("search=%+v error=%v", search, err)
	}
	requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/social/users/"+bob.User.ID+"/follow", alice.AccessToken, nil, aliceDevice)
	profileBody := requestJSONDevice(t, http.MethodGet, httpServer.URL+"/api/v1/social/users/"+bob.User.ID, alice.AccessToken, nil, aliceDevice)
	var profile struct {
		Data SocialProfile `json:"data"`
	}
	_ = json.Unmarshal(profileBody, &profile)
	if !profile.Data.Following {
		t.Fatalf("profile=%+v", profile.Data)
	}

	conversationBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/social/conversations", alice.AccessToken, map[string]string{"user_id": bob.User.ID}, aliceDevice)
	var conversationResponse struct {
		Data Conversation `json:"data"`
	}
	if err := json.Unmarshal(conversationBody, &conversationResponse); err != nil {
		t.Fatal(err)
	}
	conversation := conversationResponse.Data
	ticketBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/social/socket-ticket", bob.AccessToken, nil, bobDevice)
	var ticketResponse struct {
		Data struct {
			Ticket string `json:"ticket"`
		} `json:"data"`
	}
	_ = json.Unmarshal(ticketBody, &ticketResponse)
	wsURL := strings.Replace(httpServer.URL, "http://", "ws://", 1) + "/ws/v1/social?ticket=" + ticketResponse.Data.Ticket
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	socket, _, err := websocket.Dial(readCtx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.CloseNow()
	readTestEnvelope(t, readCtx, socket, "social.unread")
	messageBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/social/conversations/"+strconvFormat(conversation.ID)+"/messages", alice.AccessToken, map[string]string{"body": "hello Bob"}, aliceDevice)
	var sent struct {
		Data DirectMessage `json:"data"`
	}
	_ = json.Unmarshal(messageBody, &sent)
	if sent.Data.Body != "hello Bob" {
		t.Fatalf("message=%+v", sent.Data)
	}
	realtime := readTestEnvelope(t, readCtx, socket, "social.message")
	var realtimeMessage DirectMessage
	_ = json.Unmarshal(realtime.Payload, &realtimeMessage)
	if realtimeMessage.ID != sent.Data.ID {
		t.Fatalf("realtime=%+v", realtimeMessage)
	}
	unreadBody := requestJSONDevice(t, http.MethodGet, httpServer.URL+"/api/v1/social/unread", bob.AccessToken, nil, bobDevice)
	var unread struct {
		Data struct {
			Count int `json:"count"`
		} `json:"data"`
	}
	_ = json.Unmarshal(unreadBody, &unread)
	if unread.Data.Count != 1 {
		t.Fatalf("unread=%+v", unread)
	}
	messagesBody := requestJSONDevice(t, http.MethodGet, httpServer.URL+"/api/v1/social/conversations/"+strconvFormat(conversation.ID)+"/messages", bob.AccessToken, nil, bobDevice)
	var messages struct {
		Data struct {
			Messages []DirectMessage `json:"messages"`
		} `json:"data"`
	}
	_ = json.Unmarshal(messagesBody, &messages)
	if len(messages.Data.Messages) != 1 {
		t.Fatalf("messages=%+v", messages)
	}
	unreadBody = requestJSONDevice(t, http.MethodGet, httpServer.URL+"/api/v1/social/unread", bob.AccessToken, nil, bobDevice)
	_ = json.Unmarshal(unreadBody, &unread)
	if unread.Data.Count != 0 {
		t.Fatalf("read unread=%+v", unread)
	}
	if err := repository.BlockUser(ctx, bob.User.ID, alice.User.ID); err != nil {
		t.Fatal(err)
	}
	status := requestStatusDevice(t, http.MethodPost, httpServer.URL+"/api/v1/social/conversations/"+strconvFormat(conversation.ID)+"/messages", alice.AccessToken, map[string]string{"body": "blocked"}, aliceDevice)
	if status != http.StatusForbidden {
		t.Fatalf("blocked send status=%d", status)
	}
}
