package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDanmakuPersistsByMediaFingerprintAndHonorsBlocks(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	owner := AccountRecord{User: User{ID: "owner", DisplayName: "Owner", Email: "owner@example.test"}}
	member := AccountRecord{User: User{ID: "member", DisplayName: "Member", Email: "member@example.test"}}
	if err := repository.CreateUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateUser(ctx, member); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Repository: repository})
	room, err := server.store.CreateRoom(owner.User, "Danmaku room", "https://example.com/movie.mp4", 4)
	if err != nil {
		t.Fatal(err)
	}
	room, err = server.store.JoinRoom(room.Code, member.User)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	if err := repository.BlockUser(ctx, member.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	ownerClient := &socketClient{user: owner.User, send: make(chan []byte, 2)}
	memberClient := &socketClient{user: member.User, send: make(chan []byte, 2)}
	unsubscribeOwner := server.hub.Subscribe(room.Code, ownerClient)
	defer unsubscribeOwner()
	unsubscribeMember := server.hub.Subscribe(room.Code, memberClient)
	defer unsubscribeMember()
	payload, _ := json.Marshal(map[string]any{
		"body": "Hello screen", "position_seconds": 0, "color": 0xffffff, "mode": "scroll",
	})
	server.handleDanmakuMessage(ctx, ownerClient, room.Code, owner.User, payload)
	select {
	case encoded := <-ownerClient.send:
		var envelope Envelope
		if err := json.Unmarshal(encoded, &envelope); err != nil || envelope.Type != "danmaku.message" {
			t.Fatalf("envelope=%+v error=%v", envelope, err)
		}
	default:
		t.Fatal("sender did not receive danmaku")
	}
	select {
	case leaked := <-memberClient.send:
		t.Fatalf("danmaku leaked across block: %s", leaked)
	default:
	}
	messages, err := repository.ListDanmaku(ctx, mediaFingerprint(room, 0), owner.ID, 0, 10, 100)
	if err != nil || len(messages) != 1 || messages[0].Body != "Hello screen" {
		t.Fatalf("messages=%+v error=%v", messages, err)
	}
	memberMessages, err := repository.ListDanmaku(ctx, mediaFingerprint(room, 0), member.ID, 0, 10, 100)
	if err != nil || len(memberMessages) != 0 {
		t.Fatalf("blocked history=%+v error=%v", memberMessages, err)
	}
	if other := mediaFingerprint(room, 1); other == messages[0].Fingerprint {
		t.Fatal("episode was not included in media fingerprint")
	}
}

func TestMetadataSearchUsesBearerAndReturnsSafeProjection(t *testing.T) {
	const token = "metadata-token-1234567890"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/search/multi" || r.Header.Get("Authorization") != "Bearer "+token ||
			r.URL.Query().Get("query") != "Spirited Away" ||
			r.URL.Query().Get("language") != "zh-CN" || r.URL.Query().Get("include_adult") != "false" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
			{
				"id": 1, "media_type": "movie", "title": "千与千寻", "original_title": "Spirited Away",
				"overview": "Overview", "release_date": "2001-07-20", "poster_path": "/poster.jpg",
				"vote_average": 11,
			},
			{"id": 2, "media_type": "tv", "name": "Series", "first_air_date": "2020-01-01"},
			{"id": 3, "media_type": "person", "name": "Hidden"},
			{"id": 4, "media_type": "movie", "title": "Adult", "adult": true},
		}})
	}))
	defer upstream.Close()
	client, err := NewMetadataClient(token, upstream.URL+"/3/")
	if err != nil {
		t.Fatal(err)
	}
	results, err := client.Search(context.Background(), "Spirited Away", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Title != "千与千寻" ||
		results[0].PosterURL != "https://image.tmdb.org/t/p/w500/poster.jpg" ||
		results[0].Rating != 10 || results[1].MediaType != "tv" {
		t.Fatalf("results=%+v", results)
	}
}

func TestMetadataResponseLimitAndTokenValidation(t *testing.T) {
	if _, err := NewMetadataClient("short"); err == nil {
		t.Fatal("short metadata token was accepted")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"results":[]}`+strings.Repeat(" ", (4<<20)+1))
	}))
	defer upstream.Close()
	client, err := NewMetadataClient("metadata-token-1234567890", upstream.URL+"/3/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Search(context.Background(), "query", "en-US"); err == nil {
		t.Fatal("oversized metadata response was accepted")
	}
}
