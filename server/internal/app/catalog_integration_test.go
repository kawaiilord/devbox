package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCatalogPostgresRestartAndAtomicDocument(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("integration database not configured")
	}
	ctx := context.Background()
	repo, e := OpenPostgres(ctx, databaseURL)
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	if e = repo.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	repo.pool.Exec(ctx, "DELETE FROM admin_documents WHERE key='media-catalog'")
	defer repo.pool.Exec(ctx, "DELETE FROM admin_documents WHERE key='media-catalog'")
	id := "cat-admin-" + mustRandomString(8)
	account := AccountRecord{User: User{ID: id, Email: id + "@example.test", DisplayName: "Catalog test", IsAdmin: true}, PasswordHash: "test", CreatedAt: time.Now()}
	if e = repo.CreateUser(ctx, account); e != nil {
		t.Fatal(e)
	}
	defer repo.pool.Exec(ctx, "DELETE FROM users WHERE id=$1", id)
	repo.VerifyEmail(ctx, id)
	vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	c := NewCatalogService(repo, vault)
	token, _, e := c.preview(ctx, id, catalogPreviewInput{Name: "持久化", Content: `[{"name":"A","url":"https://cms.example/api"}]`})
	if e != nil {
		t.Fatal(e)
	}
	if e = c.commit(ctx, id, token); e != nil {
		t.Fatal(e)
	}
	st, version, e := c.read(ctx)
	if e != nil {
		t.Fatal(e)
	}
	src := catalogSources(st, true)[0]
	bound, e := c.bind(ctx, account.User, src)
	if e != nil {
		t.Fatal(e)
	}
	if e = repo.Migrate(ctx); e != nil {
		t.Fatalf("restart rejected new source type: %v", e)
	}
	if _, e = repo.pool.Exec(ctx, platformServicesMigration); e != nil {
		t.Fatalf("compatibility baseline cannot roll back: %v", e)
	}
	persisted, e := repo.GetMediaSource(ctx, id, bound.ID)
	if e != nil || persisted.Type != "catalog" {
		t.Fatal("source lost on migration")
	}
	// Exactly one competing writer succeeds. No last-writer-wins data loss.
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- c.save(ctx, st, version, id, "concurrent") }()
	}
	a, b := <-errs, <-errs
	if (a == nil) == (b == nil) || (!errors.Is(a, ErrAdminConflict) && !errors.Is(b, ErrAdminConflict)) {
		t.Fatalf("CAS %v %v", a, b)
	}
	recovered, _, e := NewCatalogService(repo, vault).read(ctx)
	if e != nil || len(recovered.Subscriptions) != 1 {
		t.Fatal("encrypted catalog failed after restart")
	}
}

func TestCatalogTicketProxyPlaylistAndTwoNodeRecovery(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("integration redis not configured")
	}
	ctx := context.Background()
	f, c := catalogFixture(t)
	a, e := OpenRedis(ctx, redisURL, "catalog-a")
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	a.prefix = "catalog-test-" + mustRandomString(8)
	b, e := OpenRedis(ctx, redisURL, "catalog-b")
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	b.prefix = a.prefix
	f.server.redis = a
	f.server.options.RequireAccount = true
	src := installTestCatalog(t, f, c)
	transport := discoveryTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "cms.example" {
			return discoveryReply(testCMSReply()), nil
		}
		if strings.HasSuffix(r.URL.Path, ".ts") {
			return discoveryReply("test-media-segment"), nil
		}
		return discoveryReply("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nclip.ts\n#EXT-X-ENDLIST"), nil
	})
	c.client.Transport = transport
	sourceManager := NewMediaSourceManager(f.repo, c.vault, false)
	other := NewServer(Options{Repository: f.repo, Auth: f.auth, Redis: b, Sources: sourceManager, Vault: c.vault, CatalogTransport: transport, RequireAccount: true})
	film, e := c.detail(ctx, src, "12")
	if e != nil {
		t.Fatal(e)
	}
	selection := CatalogSelection{Refs: []CatalogRef{{src.ID, "12", film.Groups[0].ID}, {src.ID, "12", film.Groups[1].ID}}, EpisodeID: film.Groups[0].Episodes[0].ID, AllEpisodes: true}
	data := f.call(t, "POST", "/api/v1/rooms", map[string]any{"name": "Ticket room", "catalog": selection}, f.member.AccessToken, 201)
	var room Room
	json.Unmarshal(data, &room)
	if !strings.Contains(room.SourceURL, "/stream.m3u8") {
		t.Fatalf("missing HLS ticket: %s", room.SourceURL)
	}
	request := httptest.NewRequest("GET", room.SourceURL, nil)
	w := httptest.NewRecorder()
	other.Handler().ServeHTTP(w, request)
	if w.Code != 200 || strings.Contains(w.Body.String(), "media.example") {
		t.Fatalf("manifest projection: %d %s", w.Code, w.Body.String())
	}
	asset := ""
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if strings.HasPrefix(line, "/media/") {
			asset = line
		}
	}
	if asset == "" {
		t.Fatal("manifest missing encrypted child asset")
	}
	w = httptest.NewRecorder()
	other.Handler().ServeHTTP(w, httptest.NewRequest("GET", asset, nil))
	if w.Code != 200 || w.Body.String() != "test-media-segment" {
		t.Fatalf("segment %d %s", w.Code, w.Body.String())
	}
	// A registered nonmember cannot edit the room or trigger owner recovery.
	f.call(t, "POST", "/api/v1/rooms/"+room.Code+"/catalog", map[string]any{"version": room.Features.Version, "selection": selection}, f.root.AccessToken, 403)
	appendData := f.call(t, "POST", "/api/v1/rooms/"+room.Code+"/catalog", map[string]any{"version": room.Features.Version, "selection": selection}, f.member.AccessToken, 200)
	json.Unmarshal(appendData, &room)
	if len(room.Features.Playlist) != 2 {
		t.Fatal("complete playlist not appended")
	}
	before := room
	payload, _ := json.Marshal(map[string]any{"version": room.Features.Version, "source_version": room.Playback.SourceVersion, "item_id": room.Features.ActiveItemID, "attempted": []string{}})
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for _, server := range []*Server{f.server, other} {
		wg.Add(1)
		go func(server *Server) {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/api/v1/rooms/"+room.Code+"/playback/recover", strings.NewReader(string(payload)))
			r.Header.Set("Authorization", "Bearer "+f.member.AccessToken)
			r.Header.Set("X-Device-ID", f.device.ID)
			w := httptest.NewRecorder()
			server.Handler().ServeHTTP(w, r)
			codes <- w.Code
		}(server)
	}
	wg.Wait()
	one, two := <-codes, <-codes
	if !((one == 200 && two == 409) || (two == 200 && one == 409)) {
		t.Fatalf("two-node switch not serialized: %d %d", one, two)
	}
	w = httptest.NewRecorder()
	other.Handler().ServeHTTP(w, httptest.NewRequest("GET", before.SourceURL, nil))
	if w.Code != 401 {
		t.Fatalf("old source version ticket retained: %d", w.Code)
	}
}
