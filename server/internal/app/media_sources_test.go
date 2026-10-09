package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestWebDAVCredentialsStayVaultedAndRangeIsForwarded(t *testing.T) {
	webdav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "viewer" || password != "top-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case "PROPFIND":
			if r.Header.Get("Depth") != "1" {
				http.Error(w, "depth required", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<?xml version="1.0"?>
<D:multistatus xmlns:D="DAV:">
 <D:response><D:href>/dav/</D:href><D:propstat><D:prop>
  <D:displayname>dav</D:displayname><D:resourcetype><D:collection/></D:resourcetype>
 </D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>
 <D:response><D:href>/dav/movie.mp4</D:href><D:propstat><D:prop>
  <D:displayname>movie.mp4</D:displayname><D:resourcetype/>
  <D:getcontentlength>10</D:getcontentlength><D:getcontenttype>video/mp4</D:getcontenttype>
 </D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>
</D:multistatus>`)
		case http.MethodGet:
			if r.Header.Get("Range") != "bytes=0-3" {
				http.Error(w, "range missing", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Range", "bytes 0-3/10")
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "test")
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	defer webdav.Close()

	key := make([]byte, 32)
	for index := range key {
		key[index] = byte(index + 3)
	}
	vault, err := NewCredentialVault(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	repository := NewMemoryRepository()
	manager := NewMediaSourceManager(repository, vault, true)
	user := User{ID: "media-user", DisplayName: "Media User"}
	source, err := manager.CreateWebDAV(
		context.Background(), user, "Home media", webdav.URL+"/dav/", "viewer", "top-secret",
	)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repository.GetMediaSource(context.Background(), user.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CredentialsCiphertext == "" || strings.Contains(stored.CredentialsCiphertext, "top-secret") {
		t.Fatalf("credentials were not encrypted: %q", stored.CredentialsCiphertext)
	}
	files, err := manager.Browse(context.Background(), user.ID, source.ID, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "movie.mp4" || files[0].Path != "/movie.mp4" {
		t.Fatalf("files=%+v", files)
	}
	response, err := manager.Open(
		context.Background(),
		MediaTicket{UserID: user.ID, SourceID: source.ID, Path: files[0].Path},
		http.MethodGet,
		"bytes=0-3",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusPartialContent || string(body) != "test" ||
		response.Header.Get("Content-Range") != "bytes 0-3/10" {
		t.Fatalf("status=%d headers=%v body=%q", response.StatusCode, response.Header, body)
	}
	if _, err := resolveSourcePath(source.BaseURL, "/../secret", false); err == nil {
		t.Fatal("path traversal was accepted")
	}
}

func TestWebDAVRejectsPrivateAddressByDefault(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	vault, _ := NewCredentialVault(key)
	manager := NewMediaSourceManager(NewMemoryRepository(), vault, false)
	_, err := manager.CreateWebDAV(
		context.Background(),
		User{ID: "user"},
		"Private",
		"https://127.0.0.1/dav/",
		"viewer",
		"secret",
	)
	if err == nil || !strings.Contains(err.Error(), "private or reserved") {
		t.Fatalf("private source error=%v", err)
	}
}

func TestMediaRoomIssuesRenewableTicketToMember(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL is not configured")
	}
	webdav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, _ := r.BasicAuth()
		if username != "viewer" || password != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method == "PROPFIND" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><D:multistatus xmlns:D="DAV:">
<D:response><D:href>/dav/</D:href><D:propstat><D:prop><D:displayname>dav</D:displayname><D:resourcetype><D:collection/></D:resourcetype></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>
<D:response><D:href>/dav/movie.srt</D:href><D:propstat><D:prop><D:displayname>movie.srt</D:displayname><D:resourcetype/><D:getcontentlength>42</D:getcontentlength><D:getcontenttype>application/x-subrip</D:getcontenttype></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>
</D:multistatus>`)
			return
		}
		if r.Header.Get("Range") != "bytes=4-7" {
			http.Error(w, "range", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Range", "bytes 4-7/10")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "data")
	}))
	defer webdav.Close()
	ctx, cancel := context.WithCancel(context.Background())
	redisNode, err := OpenRedis(ctx, redisURL, "media-room-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = redisNode.Close()
	}()
	if err := redisNode.FlushTestNamespace(ctx); err != nil {
		t.Fatal(err)
	}
	repository := NewMemoryRepository()
	tokens, _ := NewTokenManager("media-room-test-secret-with-at-least-32-characters", "media-room-test")
	auth := NewAuthService(repository, tokens)
	owner, err := auth.Register(ctx, "media-owner@example.com", "Owner", "owner strong password")
	if err != nil {
		t.Fatal(err)
	}
	member, err := auth.Register(ctx, "media-member@example.com", "Member", "member strong password")
	if err != nil {
		t.Fatal(err)
	}
	vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	manager := NewMediaSourceManager(repository, vault, true)
	source, err := manager.CreateWebDAV(
		ctx, owner.User, "Media", webdav.URL+"/dav/", "viewer", "secret",
	)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	room, err := store.CreateMediaRoom(owner.User, "Shared media", source.ID, "/movie.mp4", 4)
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
	if err := redisNode.InitializeRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{
		Repository:     repository,
		Auth:           auth,
		InitialRooms:   []Room{room},
		Redis:          redisNode,
		Sources:        manager,
		AllowedOrigins: []string{"*"},
	})
	server.StartBackground(ctx)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	ticketBody := requestJSON(
		t,
		http.MethodPost,
		httpServer.URL+"/api/v1/rooms/"+room.Code+"/media-ticket",
		member.AccessToken,
		nil,
	)
	var ticketResponse struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ticketBody, &ticketResponse); err != nil {
		t.Fatal(err)
	}
	base, _ := url.Parse(httpServer.URL)
	playURL := base.ResolveReference(&url.URL{Path: ticketResponse.Data.URL}).String()
	request, _ := http.NewRequest(http.MethodGet, playURL, nil)
	request.Header.Set("Range", "bytes=4-7")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusPartialContent || string(body) != "data" {
		t.Fatalf("status=%d body=%q", response.StatusCode, body)
	}
	subtitleBody := requestJSON(
		t,
		http.MethodGet,
		httpServer.URL+"/api/v1/rooms/"+room.Code+"/subtitles",
		member.AccessToken,
		nil,
	)
	var subtitleResponse struct {
		Data struct {
			Files []MediaFile `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(subtitleBody, &subtitleResponse); err != nil {
		t.Fatal(err)
	}
	if len(subtitleResponse.Data.Files) != 1 || subtitleResponse.Data.Files[0].Path != "/movie.srt" {
		t.Fatalf("subtitles=%+v", subtitleResponse.Data.Files)
	}
	subtitleTicketBody := requestJSON(
		t,
		http.MethodPost,
		httpServer.URL+"/api/v1/rooms/"+room.Code+"/subtitle-ticket",
		member.AccessToken,
		map[string]string{"path": "/movie.srt"},
	)
	var subtitleTicket struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(subtitleTicketBody, &subtitleTicket); err != nil || subtitleTicket.Data.URL == "" {
		t.Fatalf("subtitle ticket=%+v error=%v", subtitleTicket, err)
	}
}
