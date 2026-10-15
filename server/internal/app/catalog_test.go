package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func catalogFixture(t *testing.T) (*managementFixture, *CatalogService) {
	f := newManagementFixture(t, NewMemoryRepository())
	return f, f.server.catalog
}
func TestCatalogParsersPreviewAndCredentialRedaction(t *testing.T) {
	f, c := catalogFixture(t)
	ctx := context.Background()
	content := `{"sites":[{"key":"a","name":"A","type":1,"api":"https://video.example/api.php/provide/vod/?token=private"},{"name":"duplicate","type":1,"api":"https://video.example/api.php/provide/vod/?ac=detail&token=private"},{"name":"spider","type":3,"api":"csp_fake"}]}`
	token, update, e := c.preview(ctx, f.root.User.ID, catalogPreviewInput{Name: "首份订阅", Content: content})
	if e != nil || update.Added != 1 || update.Duplicates != 1 || len(update.Skipped) != 1 {
		t.Fatalf("preview %+v %v", update, e)
	}
	st, _, _ := c.read(ctx)
	if len(st.Subscriptions) != 0 {
		t.Fatal("preview changed active catalog")
	}
	if c.commit(ctx, f.member.User.ID, token) == nil {
		t.Fatal("preview transferred to another user")
	}
	if e = c.commit(ctx, f.root.User.ID, token); e != nil {
		t.Fatal(e)
	}
	d, _ := f.repo.GetAdminDocument(ctx, "media-catalog")
	if strings.Contains(string(d.Data), "private") || strings.Contains(string(d.Data), "video.example") {
		t.Fatal("subscription secrets stored in plaintext")
	}
	body := f.call(t, "GET", "/api/v1/admin/catalog", nil, f.root.AccessToken, 200)
	if strings.Contains(string(body), "private") || strings.Contains(string(body), "video.example") {
		t.Fatal("admin projection leaked source credentials")
	}
	f.call(t, "GET", "/api/v1/admin/catalog", nil, f.member.AccessToken, 403)
	f.call(t, "GET", "/api/v1/catalog/sources", nil, "", 401)
	token, _, e = c.preview(ctx, f.root.User.ID, catalogPreviewInput{Name: "共享引用", Content: content})
	if e != nil {
		t.Fatal(e)
	}
	if e = c.commit(ctx, f.root.User.ID, token); e != nil {
		t.Fatal(e)
	}
	st, _, _ = c.read(ctx)
	if len(catalogSources(st, true)) != 1 || len(st.Subscriptions) != 2 {
		t.Fatal("cross subscription deduplication failed")
	}
	f.call(t, "POST", "/api/v1/admin/catalog/manage", map[string]any{"revision": st.Revision, "action": "delete", "ids": []string{st.Subscriptions[0].ID}}, f.root.AccessToken, 200)
	st, _, _ = c.read(ctx)
	if len(catalogSources(st, true)) != 1 {
		t.Fatal("deleting one subscription deleted a shared source")
	}
}
func TestCatalogM3UAndUnsafeInputs(t *testing.T) {
	m3u := "#EXTM3U\n#EXTINF:-1 tvg-id=\"a\" group-title=\"测试\",演示频道\nhttps://cdn.example/a.m3u8\n#EXTINF:-1,重复\nhttps://cdn.example/a.m3u8\n#EXTINF:-1,私网\nhttp://127.0.0.1/admin\n"
	channels, skips, e := parseCatalogM3U([]byte(m3u), "")
	if e != nil || len(channels) != 1 || skips != 2 {
		t.Fatalf("M3U %+v %d %v", channels, skips, e)
	}
	for _, raw := range []string{"http://localhost/config", "http://127.0.0.1/", "http://[::1]/", "http://192.168.1.1/", "file:///etc/passwd", "https://user:password@example.com/", "https://a.example/x#fragment"} {
		if _, e := catalogURL(raw); e == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
	if _, _, e = parseCatalogM3U([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\na.ts"), "https://cdn.example/live.m3u8"); e == nil {
		t.Fatal("HLS media manifest imported as channel list")
	}
	_, c := catalogFixture(t)
	for _, raw := range []string{`{"sites":[{"name":"Spider","type":3,"api":"csp_script"}]}`, `{broken`, strings.Repeat("a", catalogMaxBytes+1)} {
		if _, _, _, e := c.parseImport(context.Background(), []byte(raw), "", "bad"); e == nil {
			t.Fatal("invalid list accepted")
		}
	}
	// Even a public-looking host cannot redirect to loopback. The transport also
	// checks DNS on every dial, rather than trusting the import-time hostname.
	r := httptest.NewRequest("GET", "http://127.0.0.1/secret", nil)
	if c.client.CheckRedirect(r, []*http.Request{httptest.NewRequest("GET", "https://public.example/", nil)}) == nil {
		t.Fatal("unsafe redirect accepted")
	}
}
func TestCatalogSchedulerLastGoodCASAndRestart(t *testing.T) {
	f, c := catalogFixture(t)
	ctx := context.Background()
	var fail atomic.Bool
	var calls atomic.Int32
	c.client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if fail.Load() {
			return nil, errors.New("offline")
		}
		return discoveryReply(`{"sources":[{"name":"A","url":"https://video.example/api"}]}`), nil
	})
	token, _, e := c.preview(ctx, f.root.User.ID, catalogPreviewInput{Name: "自动订阅", URL: "https://feed.example/catalog.json", IntervalHours: 1})
	if e != nil {
		t.Fatal(e)
	}
	if e = c.commit(ctx, f.root.User.ID, token); e != nil {
		t.Fatal(e)
	}
	token, _, e = c.preview(ctx, f.root.User.ID, catalogPreviewInput{Name: "等待确认", Content: `[{"name":"B","url":"https://other.example/api"}]`})
	if e != nil {
		t.Fatal(e)
	}
	_ = c.mutate(ctx, "", "test", func(st *CatalogState) error { st.Subscriptions[0].NextRun = 0; return nil })
	fail.Store(true)
	if e = c.refreshDue(ctx); e != nil {
		t.Fatal(e)
	}
	st, _, _ := c.read(ctx)
	if len(catalogSources(st, true)) != 1 || st.Subscriptions[0].Error == "" || st.History[0].Removed != 0 {
		t.Fatalf("last good state lost %+v", st.History)
	}
	fail.Store(false)
	_ = c.mutate(ctx, "", "test", func(st *CatalogState) error { st.Subscriptions[0].NextRun = 0; return nil })
	if e = c.refreshDue(ctx); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(c.commit(ctx, f.root.User.ID, token), ErrAdminConflict) {
		t.Fatal("stale preview overwrote scheduled update")
	}
	restored := NewCatalogService(f.repo, c.vault)
	st, _, e = restored.read(ctx)
	if e != nil || len(st.Subscriptions) != 1 || st.Subscriptions[0].Error != "" {
		t.Fatal("catalog did not survive service restart")
	}
	if e = c.refreshDue(ctx); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 3 {
		t.Fatalf("scheduler ignored next due time: %d", calls.Load())
	}
}
func installTestCatalog(t *testing.T, f *managementFixture, c *CatalogService) CatalogSource {
	t.Helper()
	token, _, e := c.preview(context.Background(), f.root.User.ID, catalogPreviewInput{Name: "测试接口", Content: `[{"name":"测试源","url":"https://cms.example/api"}]`})
	if e != nil {
		t.Fatal(e)
	}
	if e = c.commit(context.Background(), f.root.User.ID, token); e != nil {
		t.Fatal(e)
	}
	st, _, _ := c.read(context.Background())
	return catalogSources(st, true)[0]
}
func testCMSReply() string {
	return `{"code":1,"pagecount":2,"list":[{"vod_id":12,"vod_name":"测试电影","vod_year":"2026","type_name":"电影","vod_content":"<p>简介</p>","vod_play_from":"主线$$$备用","vod_play_url":"正片$https://media.example/primary.m3u8$$$正片$https://media.example/backup.m3u8"}]}`
}
func TestCatalogDetailsPlaylistDisableAndRoomRecovery(t *testing.T) {
	f, c := catalogFixture(t)
	ctx := context.Background()
	src := installTestCatalog(t, f, c)
	c.client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "cms.example" {
			return discoveryReply(testCMSReply()), nil
		}
		return discoveryReply("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nclip.ts\n#EXT-X-ENDLIST"), nil
	})
	res, e := c.search(ctx, src, "测试电影", 1)
	if e != nil || len(res.Items) != 1 || !res.HasMore {
		t.Fatalf("search %+v %v", res, e)
	}
	film, e := c.detail(ctx, src, "12")
	if e != nil || len(film.Groups) != 2 || film.Summary != "简介" {
		t.Fatalf("detail %+v %v", film, e)
	}
	encoded, _ := json.Marshal(film)
	if strings.Contains(string(encoded), "media.example") {
		t.Fatal("raw media URL leaked")
	}
	selection := CatalogSelection{Refs: []CatalogRef{{src.ID, "12", film.Groups[0].ID}, {src.ID, "12", film.Groups[1].ID}}, EpisodeID: film.Groups[0].Episodes[0].ID, AllEpisodes: true}
	data := f.call(t, "POST", "/api/v1/rooms", map[string]any{"name": "测试播放", "catalog": selection}, f.member.AccessToken, 201)
	var room Room
	json.Unmarshal(data, &room)
	saved, features, e := f.server.authorizedRoom(ctx, room.Code, f.member.User)
	if e != nil {
		t.Fatal(e)
	}
	if len(features.Playlist) != 1 || len(features.Playlist[0].Sources) != 2 || !strings.HasPrefix(saved.MediaSourceID, "cat_") {
		t.Fatalf("invalid generated room %+v", features)
	}
	// Restore a paused, non-zero authoritative position; changing source must not seek to 0.
	saved.Playback.Position = 84
	saved.Playback.Playing = false
	saved.Playback.PositionTS = time.Now().UnixMilli()
	f.server.store.UpsertAuthoritativeRoom(saved, 0)
	if e = f.repo.SaveRoom(ctx, saved); e != nil {
		t.Fatal(e)
	}
	recovery := map[string]any{"version": features.Version, "source_version": saved.Playback.SourceVersion, "item_id": features.ActiveItemID, "attempted": []string{}}
	f.call(t, "POST", "/api/v1/rooms/"+room.Code+"/playback/recover", recovery, f.member.AccessToken, 200)
	changed, features, e := f.server.authorizedRoom(ctx, room.Code, f.member.User)
	if e != nil {
		t.Fatal(e)
	}
	if changed.MediaPath == saved.MediaPath || changed.Playback.Position != 84 || changed.Playback.Episode != saved.Playback.Episode {
		t.Fatalf("recovery lost identity/position %+v", changed.Playback)
	}
	f.call(t, "POST", "/api/v1/rooms/"+room.Code+"/playback/recover", recovery, f.member.AccessToken, 409)
	ticket, e := f.server.sources.PrepareMediaTicket(ctx, f.member.User.ID, changed.MediaSourceID, changed.MediaPath)
	if e != nil || ticket.Kind != "platform-hls" {
		t.Fatalf("ticket %+v %v", ticket, e)
	}
	st, _, _ := c.read(ctx)
	f.call(t, "POST", "/api/v1/admin/catalog/manage", map[string]any{"revision": st.Revision, "action": "source-toggle", "ids": []string{src.ID}, "enabled": false}, f.root.AccessToken, 200)
	if _, e = f.server.sources.Open(ctx, ticket, "GET", "", ""); e == nil {
		t.Fatal("disabled catalog source still served existing ticket")
	}
}
func TestCatalogGroupingAndCooldown(t *testing.T) {
	if catalogGroupKey("Example", "2020", "movie", "a", "1") == catalogGroupKey("Example", "2024", "movie", "b", "1") {
		t.Fatal("remakes merged")
	}
	if catalogGroupKey("Show 1", "2020", "tv", "a", "1") == catalogGroupKey("Show 2", "2020", "tv", "a", "2") {
		t.Fatal("seasons merged")
	}
	if catalogGroupKey("Example", "", "movie", "a", "1") == catalogGroupKey("Example", "", "movie", "b", "1") {
		t.Fatal("unknown years merged")
	}
	if catalogEpisodeKey(CatalogEpisode{Name: "第01集"}, "tv") != catalogEpisodeKey(CatalogEpisode{Name: "1"}, "tv") {
		t.Fatal("equivalent episodes not aligned")
	}
	f, c := catalogFixture(t)
	src := installTestCatalog(t, f, c)
	ctx := context.Background()
	for range 3 {
		c.health(ctx, src.ID, c.now(), fmt.Errorf("failure"))
	}
	st, _, _ := c.read(ctx)
	if st.Health[src.ID].SuspendedUntil <= c.now().UnixMilli() {
		t.Fatal("unhealthy source not cooled down")
	}
	c.health(ctx, src.ID, c.now(), nil)
	st, _, _ = c.read(ctx)
	if st.Health[src.ID].Failures != 0 || st.Health[src.ID].SuspendedUntil != 0 {
		t.Fatal("successful probe did not restore source")
	}
}

func TestCatalogMediaKindDoesNotTreatDramaFilmsAsSeries(t *testing.T) {
	for input, expected := range map[string]string{
		"剧情片": "movie", "喜剧片": "movie", "剧情": "movie", "动画电影": "movie",
		"动作片": "movie", "国产剧": "tv", "欧美剧": "tv", "日本剧": "tv",
		"连续剧": "tv", "日本动漫": "anime", "大陆综艺": "variety", "短剧": "short",
	} {
		if got := catalogMediaKind(input); got != expected {
			t.Errorf("%q: got %q, want %q", input, got, expected)
		}
	}
}
