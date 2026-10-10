package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type quarkTestTransport func(*http.Request) (*http.Response, error)

func (f quarkTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func quarkTestManager(repository Repository, handler http.HandlerFunc) *MediaSourceManager {
	vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	manager := NewMediaSourceManager(repository, vault, false)
	manager.quarkClient = &http.Client{Transport: quarkTestTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		handler(w, r)
		response := w.Result()
		response.Request = r
		return response, nil
	})}
	return manager
}

func quarkSuccess(w http.ResponseWriter, data any, total int) {
	writeTestJSON(w, map[string]any{"code": 0, "status": 200, "data": data, "metadata": map[string]int{"_total": total}})
}

func quarkFixture(t *testing.T, resolveCount *atomic.Int64) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host == "drive.quark.cn" {
			if !strings.Contains(r.Header.Get("Cookie"), "__pus=owner-secret") || r.Header.Get("Referer") != quarkWebsite ||
				r.URL.Query().Get("pr") != "ucpro" || r.URL.Query().Get("fr") != "pc" {
				t.Errorf("invalid Quark API authentication or query")
			}
			http.SetCookie(w, &http.Cookie{Name: "__puus", Value: "rotated-secret", Path: "/", Secure: true})
			switch r.URL.Path {
			case "/1/clouddrive/file/sort":
				if r.URL.Query().Get("pdir_fid") == "0" {
					quarkSuccess(w, map[string]any{"list": []quarkFile{{ID: "folder1", Name: "电影"}}}, 1)
				} else {
					quarkSuccess(w, map[string]any{"list": []quarkFile{
						{ID: "movie1", Name: "Movie &amp; Friends.mp4", File: true, Category: 1, Size: 10, ContentType: "video/mp4"},
						{ID: "document1", Name: "private.txt", File: true, ContentType: "text/plain"},
					}}, 2)
				}
			case "/1/clouddrive/file/download":
				var body struct {
					IDs []string `json:"fids"`
				}
				if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil || len(body.IDs) != 1 || body.IDs[0] != "movie1" {
					t.Errorf("invalid Quark file resolution request")
				}
				n := resolveCount.Add(1)
				quarkSuccess(w, []quarkFile{{ID: "movie1", Name: "Movie.mp4", File: true, Category: 1,
					DownloadURL: fmt.Sprintf("https://download.pan.quark.cn/movie?signature=%d", n)}}, 0)
			default:
				t.Errorf("unexpected Quark API route %s", r.URL.Path)
			}
			return
		}
		if r.URL.Host != "download.pan.quark.cn" || !strings.Contains(r.Header.Get("Cookie"), "__puus=rotated-secret") {
			t.Errorf("unexpected stream destination or stale login")
		}
		if r.Method != http.MethodHead && r.Header.Get("Range") != "bytes=4-7" {
			t.Errorf("missing playback range")
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 4-7/10")
		w.Header().Set("Set-Cookie", "must-not-reach-player=secret")
		w.WriteHeader(http.StatusPartialContent)
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, "data")
		}
	}
}

func TestQuarkLoginBrowseVaultRotationPlaybackAndRenewal(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	var resolves atomic.Int64
	manager := quarkTestManager(repo, quarkFixture(t, &resolves))
	owner := User{ID: "owner"}
	source, err := manager.SaveQuark(ctx, owner, "", "My Quark", "__pus=owner-secret; __puus=old")
	if err != nil {
		t.Fatal(err)
	}
	stored, secret, err := manager.loadSourceSecret(ctx, owner.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.CredentialsCiphertext, "secret") || !strings.Contains(string(secret), "rotated-secret") {
		t.Fatal("cookie must rotate into encrypted storage")
	}
	public, _ := json.Marshal(source)
	if strings.Contains(string(public), "secret") || strings.Contains(string(public), "cookie") {
		t.Fatal("source response leaked login")
	}
	root, err := manager.Browse(ctx, owner.ID, source.ID, "/")
	if err != nil || len(root) != 1 || root[0].Path != "/folder1" || !root[0].IsDirectory {
		t.Fatalf("root=%+v err=%v", root, err)
	}
	files, err := manager.Browse(ctx, owner.ID, source.ID, root[0].Path)
	if err != nil || len(files) != 1 || files[0].Path != "/folder1/movie1" || files[0].Name != "Movie & Friends.mp4" {
		t.Fatalf("files=%+v err=%v", files, err)
	}
	ticket, err := manager.PrepareMediaTicket(ctx, owner.ID, source.ID, files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(ticket)
	if strings.Contains(string(encoded), "signature") || strings.Contains(string(encoded), "secret") {
		t.Fatal("ticket leaked upstream link or login")
	}
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodGet} {
		response, err := manager.Open(ctx, ticket, method, "bytes=4-7", `"etag"`)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 206 || (method == http.MethodGet && string(body) != "data") {
			t.Fatal("range playback failed")
		}
	}
	if resolves.Load() != 4 {
		t.Fatalf("expired links must be re-resolved on each media request, got %d resolutions", resolves.Load())
	}
	if _, err := manager.SaveQuark(ctx, User{ID: "stranger"}, source.ID, "", "__pus=owner-secret"); !errors.Is(err, ErrNotFound) {
		t.Fatal("another user replaced the login")
	}
	updated, err := manager.SaveQuark(ctx, owner, source.ID, "ignored", "__pus=owner-secret")
	if err != nil || updated.ID != source.ID || updated.Name != source.Name {
		t.Fatal("reauthentication must preserve the source")
	}
	if ok, err := repo.ReplaceMediaSourceCredentials(ctx, owner.ID, source.ID, stored.CredentialsCiphertext, "stale"); err != nil || ok {
		t.Fatal("stale rotation overwrote a newer login")
	}
	if err := manager.Delete(ctx, owner.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Open(ctx, ticket, http.MethodGet, "bytes=4-7", ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted source remained playable")
	}
}

func TestQuarkRejectsInvalidCredentialsPathsAndProviderResponses(t *testing.T) {
	for _, cookie := range []string{"", "name=value", "__pus=x\r\nInjected: secret", "__pus=x; __pus=y", "__pus="} {
		if _, err := normalizeQuarkCookie(cookie); err == nil {
			t.Errorf("accepted invalid login")
		}
	}
	for _, value := range []string{"/../id", "/folder/%2fid", "/folder//id", "/folder/..", "/0", "id", "/folder\\id"} {
		if _, err := quarkIDFromPath(value, false); err == nil {
			t.Errorf("accepted path %q", value)
		}
	}
	for _, value := range []string{"http://pan.quark.cn/file", "https://user:secret@pan.quark.cn/file", "https://127.0.0.1/file", "https://pan.quark.cn:8443/file", "https://pan.quark.cn/file#fragment"} {
		if _, err := quarkDownloadURL(value); err == nil {
			t.Errorf("accepted URL %q", value)
		}
	}
	for _, body := range []string{
		`{"code":31001,"status":401,"message":"DO-NOT-LEAK"}`,
		`{"code":12345,"status":200,"message":"DO-NOT-LEAK"}`,
		`{"status":200,"data":{}}`, `DO-NOT-LEAK`,
	} {
		repo := NewMemoryRepository()
		manager := quarkTestManager(repo, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
		_, err := manager.SaveQuark(context.Background(), User{ID: "owner"}, "", "Quark", "__pus=secret")
		if err == nil || strings.Contains(err.Error(), "DO-NOT-LEAK") {
			t.Fatalf("unsafe error: %v", err)
		}
		items, _ := repo.ListMediaSources(context.Background(), "owner")
		if len(items) != 0 {
			t.Fatal("failed login was saved")
		}
	}
}

func TestQuarkPaginationIncludesFilesBeyondFirstPage(t *testing.T) {
	repo := NewMemoryRepository()
	manager := quarkTestManager(repo, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("_page") == "2" {
			quarkSuccess(w, map[string]any{"list": []quarkFile{{ID: "last", Name: "Last.mp4", File: true, Category: 1}}}, 101)
			return
		}
		files := make([]quarkFile, 100)
		for i := range files {
			files[i] = quarkFile{ID: fmt.Sprintf("dir%d", i), Name: fmt.Sprintf("Folder%d", i)}
		}
		quarkSuccess(w, map[string]any{"list": files}, 101)
	})
	source, err := manager.SaveQuark(context.Background(), User{ID: "owner"}, "", "Quark", "__pus=secret")
	if err != nil {
		t.Fatal(err)
	}
	files, err := manager.Browse(context.Background(), "owner", source.ID, "/")
	if err != nil || len(files) != 101 || files[len(files)-1].Name != "Last.mp4" {
		t.Fatalf("pagination files=%d err=%v", len(files), err)
	}
}

func TestQuarkRedirectsStripCredentialsAndRetryExpiredLinks(t *testing.T) {
	var attempts atomic.Int64
	manager := quarkTestManager(NewMemoryRepository(), func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Host {
		case "drive.quark.cn":
			if strings.HasSuffix(r.URL.Path, "/sort") {
				quarkSuccess(w, map[string]any{"list": []quarkFile{}}, 0)
				return
			}
			quarkSuccess(w, []quarkFile{{ID: "movie", Name: "Movie.mp4", DownloadURL: "https://download.pan.quark.cn/file"}}, 0)
		case "download.pan.quark.cn":
			if !strings.Contains(r.Header.Get("Cookie"), "secret") {
				t.Error("provider lost authentication")
			}
			if attempts.Add(1) == 1 {
				w.WriteHeader(403)
				return
			}
			http.Redirect(w, r, "https://cdn.example.test/movie?signature=opaque", 302)
		case "cdn.example.test":
			if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
				t.Error("CDN received login credentials")
			}
			if r.Header.Get("Range") != "bytes=4-7" || r.Header.Get("If-Range") != `"etag"` {
				t.Error("redirect lost seek headers")
			}
			w.WriteHeader(206)
		default:
			t.Error("unexpected redirect destination")
		}
	})
	source, err := manager.SaveQuark(context.Background(), User{ID: "owner"}, "", "Quark", "__pus=secret")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := manager.PrepareMediaTicket(context.Background(), "owner", source.ID, "/movie")
	if err != nil {
		t.Fatal(err)
	}
	response, err := manager.Open(context.Background(), ticket, http.MethodGet, "bytes=4-7", `"etag"`)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 206 || attempts.Load() != 2 {
		t.Fatal("expired URL retry failed")
	}

	var requests atomic.Int64
	manager = quarkTestManager(NewMemoryRepository(), func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "https://untrusted.example.test/collect", 302)
	})
	if _, err := manager.SaveQuark(context.Background(), User{ID: "owner"}, "", "Quark", "__pus=secret"); err == nil || requests.Load() != 1 {
		t.Fatal("API redirect must not be followed")
	}
}

func TestQuarkRoomMembersCanStreamWithoutProviderLogin(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL is not configured")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	redisNode, err := OpenRedis(ctx, redisURL, "quark-room-test")
	if err != nil {
		t.Fatal(err)
	}
	defer redisNode.Close()
	if err := redisNode.FlushTestNamespace(ctx); err != nil {
		t.Fatal(err)
	}
	repo := NewMemoryRepository()
	var resolves atomic.Int64
	manager := quarkTestManager(repo, quarkFixture(t, &resolves))
	tokens, _ := NewTokenManager("quark-room-test-secret-with-more-than-32-characters", "quark-room-test")
	auth := NewAuthService(repo, tokens)
	owner, err := auth.Register(ctx, "quark-owner@example.com", "Owner", "strong owner password")
	if err != nil {
		t.Fatal(err)
	}
	member, err := auth.Register(ctx, "quark-member@example.com", "Member", "strong member password")
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := auth.Register(ctx, "quark-stranger@example.com", "Stranger", "strong stranger password")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Repository: repo, Auth: auth, Redis: redisNode, Sources: manager, AllowedOrigins: []string{"*"}})
	server.StartBackground(ctx)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	create := requestJSON(t, http.MethodPost, httpServer.URL+"/api/v1/sources/quark", owner.AccessToken, map[string]string{"name": "Quark", "cookie": "__pus=owner-secret"})
	var source struct {
		Data MediaSource `json:"data"`
	}
	if err := json.Unmarshal(create, &source); err != nil {
		t.Fatal(err)
	}
	created := requestJSON(t, http.MethodPost, httpServer.URL+"/api/v1/rooms", owner.AccessToken, map[string]any{
		"name": "Quark movie", "media_source_id": source.Data.ID, "media_path": "/folder1/movie1", "max_members": 4,
	})
	var room struct {
		Data Room `json:"data"`
	}
	if err := json.Unmarshal(created, &room); err != nil {
		t.Fatal(err)
	}
	joined := requestJSON(t, http.MethodPost, httpServer.URL+"/api/v1/rooms/"+room.Data.Code+"/join", member.AccessToken, nil)
	if err := json.Unmarshal(joined, &room); err != nil {
		t.Fatal(err)
	}
	if len(room.Data.Members) != 2 || !strings.HasPrefix(room.Data.SourceURL, "/media/") {
		t.Fatal("member did not receive a proxied room stream")
	}
	for _, payload := range [][]byte{create, created, joined} {
		if strings.Contains(string(payload), "secret") || strings.Contains(string(payload), "download.pan.quark.cn") {
			t.Fatal("room API leaked Quark credentials or direct link")
		}
	}
	request, _ := http.NewRequest(http.MethodGet, httpServer.URL+room.Data.SourceURL, nil)
	request.Header.Set("Range", "bytes=4-7")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 206 || string(body) != "data" || response.Header.Get("Set-Cookie") != "" {
		t.Fatal("member playback or cookie isolation failed")
	}
	requestJSON(t, http.MethodPost, httpServer.URL+"/api/v1/rooms/"+room.Data.Code+"/media-ticket", member.AccessToken, nil)
	socketCtx, stopSockets := context.WithTimeout(ctx, 10*time.Second)
	defer stopSockets()
	ownerSocket := dialTestRoomSocket(t, socketCtx, httpServer.URL, room.Data.Code,
		issueTestSocketTicket(t, httpServer.URL, room.Data.Code, owner.AccessToken))
	defer ownerSocket.CloseNow()
	memberSocket := dialTestRoomSocket(t, socketCtx, httpServer.URL, room.Data.Code,
		issueTestSocketTicket(t, httpServer.URL, room.Data.Code, member.AccessToken))
	defer memberSocket.CloseNow()
	readTestEnvelope(t, socketCtx, ownerSocket, "room.state")
	readTestEnvelope(t, socketCtx, memberSocket, "room.state")
	for i, action := range []string{"play", "pause", "seek"} {
		payload, _ := json.Marshal(map[string]any{"action": action, "position": 42.0})
		control, _ := json.Marshal(Envelope{Type: "playback.control", Seq: int64(i + 1), Payload: payload})
		if err := ownerSocket.Write(socketCtx, websocket.MessageText, control); err != nil {
			t.Fatal(err)
		}
		snapshot := readTestEnvelope(t, socketCtx, memberSocket, "playback.snapshot")
		var playback Playback
		if err := json.Unmarshal(snapshot.Payload, &playback); err != nil {
			t.Fatal(err)
		}
		if playback.Playing != (action == "play") || (action == "seek" && playback.Position != 42) {
			t.Fatalf("Quark room did not synchronize %s: %+v", action, playback)
		}
	}
	for _, route := range []string{"/api/v1/sources/" + source.Data.ID + "/files", "/api/v1/rooms/" + room.Data.Code + "/media-ticket"} {
		method := http.MethodGet
		if strings.HasSuffix(route, "media-ticket") {
			method = http.MethodPost
		}
		r, _ := http.NewRequest(method, httpServer.URL+route, nil)
		r.Header.Set("Authorization", "Bearer "+stranger.AccessToken)
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode < 400 {
			t.Fatal("stranger gained access to private drive or room")
		}
	}
}

func TestPostgresQuarkCredentialReplacementSurvivesMigrations(t *testing.T) {
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
	owner := User{ID: "quark-pg-" + mustRandomString(8), Email: "quark-" + mustRandomString(8) + "@example.com", DisplayName: "Quark"}
	if err := repo.CreateUser(ctx, AccountRecord{User: owner, PasswordHash: "test-hash", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	defer repo.pool.Exec(ctx, "DELETE FROM users WHERE id = $1", owner.ID)
	var resolves atomic.Int64
	manager := quarkTestManager(repo, quarkFixture(t, &resolves))
	source, err := manager.SaveQuark(ctx, owner, "", "Persistent Quark", "__pus=owner-secret")
	if err != nil {
		t.Fatal(err)
	}
	// Startup reapplies all migrations. Earlier Emby migration must retain Quark.
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	updated, err := manager.SaveQuark(ctx, owner, source.ID, "", "__pus=owner-secret")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetMediaSource(ctx, owner.ID, source.ID)
	if err != nil || stored.Type != "quark" || stored.CredentialsCiphertext != updated.CredentialsCiphertext {
		t.Fatal("Quark credential did not persist")
	}
	if ok, err := repo.ReplaceMediaSourceCredentials(ctx, owner.ID, source.ID, source.CredentialsCiphertext, "stale"); err != nil || ok {
		t.Fatal("Postgres accepted a stale credential update")
	}
	if ok, err := repo.ReplaceMediaSourceCredentials(ctx, "stranger", source.ID, updated.CredentialsCiphertext, "stolen"); err != nil || ok {
		t.Fatal("Postgres accepted another owner's credential update")
	}
}
