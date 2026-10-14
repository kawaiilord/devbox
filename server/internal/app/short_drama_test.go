package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestShortDramaAuthStableEpisodesAndPrivateMediaBoundary(t *testing.T) {
	f, c := catalogFixture(t)
	const id = "7000000000000000001"
	token := strings.Repeat("service-secret-", 4)
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing dedicated service authentication")
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/bridge/recommendations":
			io.WriteString(w, `{"data":[{"id":"hongguo:7000000000000000001","title":"短剧样本","cover":"https://image.example/poster.jpg","totalEpisode":2},{"id":"hongguo:7000000000000000001","title":"duplicate"}],"hasMore":true,"nextOffset":18,"sessionId":"feed-session"}`)
		case "/bridge/detail":
			io.WriteString(w, `{"drama":{"id":"hongguo:7000000000000000001","title":"短剧样本"},"episodes":[{"id":"11","name":"第1集"},{"id":"22","name":"第2集"}]}`)
		case "/bridge/stream.m3u8":
			io.WriteString(w, "#EXTM3U\n#EXTINF:3,\nsegment.ts?session=private&segment=0\n#EXT-X-ENDLIST\n")
		case "/bridge/segment.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			w.Write([]byte{0x47, 1, 2})
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	var err error
	c.guoguo, err = NewGuoguoService(upstream.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	f.call(t, "POST", "/api/v1/short-drama/feed", map[string]any{"section": "short_play"}, "", 401)
	if hits.Load() != 0 {
		t.Fatal("anonymous requests reached provider")
	}
	raw := f.call(t, "POST", "/api/v1/short-drama/feed", map[string]any{"section": "short_play"}, f.member.AccessToken, 200)
	var feed struct {
		Items  []CatalogFilm `json:"items"`
		Offset int           `json:"offset"`
	}
	json.Unmarshal(raw, &feed)
	if len(feed.Items) != 1 || feed.Items[0].ID != id || feed.Offset != 18 || !strings.HasPrefix(feed.Items[0].Poster, "/api/v1/short-drama/poster?") {
		t.Fatalf("bad normalized feed: %s", raw)
	}
	if strings.Contains(string(raw), token) || strings.Contains(string(raw), upstream.URL) || strings.Contains(string(raw), "image.example") {
		t.Fatal("internal service or origin exposed")
	}
	f.call(t, "POST", "/api/v1/short-drama/feed", map[string]any{"section": "unknown"}, f.member.AccessToken, 400)
	src, err := c.source(context.Background(), guoguoSourceID)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := c.detail(context.Background(), src, id)
	if err != nil || len(detail.Groups) != 1 || len(detail.Groups[0].Episodes) != 2 {
		t.Fatal("detail", detail, err)
	}
	selection := CatalogSelection{Refs: []CatalogRef{{SourceID: guoguoSourceID, FilmID: id, GroupID: "main"}}, EpisodeID: "22", AllEpisodes: true}
	items, active, err := c.playlist(context.Background(), f.member.User, selection)
	if err != nil || len(items) != 2 || active != 1 {
		t.Fatal("episode selection", items, active, err)
	}
	chosen := items[1].Sources[0]
	ticket, err := f.server.sources.PrepareMediaVariant(context.Background(), chosen.OwnerID, chosen.MediaSourceID, chosen.MediaPath, "")
	if err != nil || ticket.Kind != "platform-hls" || !strings.Contains(ticket.StreamURL, "episode=22") {
		t.Fatal("ticket", ticket, err)
	}
	res, err := f.server.sources.Open(context.Background(), ticket, "GET", "", "")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	rewritten, err := f.server.rewriteHLS("opaque-ticket", ticket.StreamURL, data, ticket.StreamURL)
	if err != nil || strings.Contains(string(rewritten), upstream.URL) || strings.Contains(string(rewritten), "session=private") {
		t.Fatal("manifest not encrypted", string(rewritten), err)
	}
	for _, line := range strings.Split(string(rewritten), "\n") {
		if !strings.HasPrefix(line, "/") {
			continue
		}
		u, _ := url.Parse(line)
		asset, err := f.server.decodePlatformAsset("opaque-ticket", u.Query().Get("asset"))
		if err != nil {
			t.Fatal(err)
		}
		assetTicket := ticket
		assetTicket.StreamURL = asset.URL
		res, err := f.server.sources.Open(context.Background(), assetTicket, "GET", "", "")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if len(b) != 3 || b[0] != 0x47 {
			t.Fatal("segment proxy failed")
		}
	}
	for _, target := range []string{"http://127.0.0.1:1/bridge/segment.ts", upstream.URL + "/api/ui/config", upstream.URL + "/bridge/../config", upstream.URL + "/bridge/stream.m3u8#fragment"} {
		if res, err := c.guoguo.openMedia(context.Background(), target, "GET", "", ""); err == nil {
			res.Body.Close()
			t.Fatal("unsafe bridge URL accepted", target)
		}
	}
	if _, err = catalogURL(upstream.URL); err == nil {
		t.Fatal("normal catalog private-address guard weakened")
	}
	c.guoguo = nil
	if _, err = c.source(context.Background(), guoguoSourceID); err == nil {
		t.Fatal("disabled provider stayed available")
	}
}

func TestShortDramaProviderRefusesRedirectAndUserInput(t *testing.T) {
	token := strings.Repeat("x", 32)
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer upstream.Close()
	g, _ := NewGuoguoService(upstream.URL, token)
	if _, e := g.json(context.Background(), "GET", "/bridge/detail", nil, nil); e == nil {
		t.Fatal("redirect accepted")
	}
	if leaked.Load() {
		t.Fatal("service token leaked via redirect")
	}
	for _, q := range []shortDramaQuery{{Section: "short_play", Offset: -1}, {Section: "search", Query: ""}, {Section: "hot", Page: 9999}, {Section: "short_play", Seen: []string{"https://private"}}} {
		if q.validate() == nil {
			t.Fatal("invalid query accepted", q)
		}
	}
	for _, endpoint := range []string{"file:///tmp/data", upstream.URL + "/admin", upstream.URL + "?key=secret"} {
		if _, e := NewGuoguoService(endpoint, token); e == nil {
			t.Fatal("invalid endpoint accepted")
		}
	}
}
