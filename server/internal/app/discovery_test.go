package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type discoveryTransport func(*http.Request) (*http.Response, error)

func (f discoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func discoveryReply(data string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(data))}
}

func TestDiscoveryRequiresAccountAndValidatesBeforeFetching(t *testing.T) {
	f := newManagementFixture(t, NewMemoryRepository())
	d := NewDiscoveryService()
	f.server.options.Discovery = d
	var calls atomic.Int64
	d.client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return discoveryReply(`{"subjects":[{"id":"123","title":"测试影片","rate":"8.8","cover":"http://img3.doubanio.com/view/photo/s_ratio_poster/public/test.jpg"}]}`), nil
	})
	request := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("X-Device-ID", f.device.ID)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		f.server.Handler().ServeHTTP(w, r)
		return w
	}
	if w := request("/api/v1/discovery", ""); w.Code != 401 {
		t.Fatalf("anonymous: %d", w.Code)
	}
	for _, query := range []string{"section=unknown", "page=0", "page=21", "page=wrong", "section=series&category=http://127.0.0.1", "section=anime&weekday=8"} {
		if w := request("/api/v1/discovery?"+query, f.member.AccessToken); w.Code != 400 {
			t.Fatalf("query %s: %d", query, w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid or anonymous requests reached the upstream")
	}
	w := request("/api/v1/discovery", f.member.AccessToken)
	if w.Code != 200 {
		t.Fatalf("feed: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Data DiscoveryFeed `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &response)
	if len(response.Data.Items) != 1 || !strings.HasPrefix(response.Data.Items[0].Poster, "/api/v1/discovery/poster?") {
		t.Fatalf("missing signed poster: %+v", response.Data)
	}
	if strings.Contains(w.Body.String(), "access_token") || strings.Contains(w.Body.String(), "refresh_token") {
		t.Fatal("account tokens leaked into discovery")
	}
}

func TestDiscoveryTVRankingFiltersAndOrdersActualScores(t *testing.T) {
	d := NewDiscoveryService()
	d.client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if r.URL.Host != "movie.douban.com" || q.Get("type") != "tv" || q.Get("tag") != "热门" || q.Get("page_limit") != "100" {
			t.Fatalf("unexpected ranking request: %s", r.URL)
		}
		return discoveryReply(`{"subjects":[{"id":"1","title":"八分","rate":"8.0"},{"id":"2","title":"九分","rate":"9.1"},{"id":"2","title":"重复","rate":"9.1"},{"id":"3","title":"低分","rate":"7.9"},{"id":"4","title":"待评分","rate":""},{"id":"../5","title":"无效标识","rate":"9.9"}]}`), nil
	})
	feed, err := d.Feed(context.Background(), "top", "tv", 1, 0)
	if err != nil || len(feed.Items) != 2 || feed.Items[0].Title != "九分" || feed.Items[0].Rank != 1 || feed.Items[1].Rank != 2 {
		t.Fatalf("ranking=%+v err=%v", feed, err)
	}
}

func TestDiscoveryCalendarUsesShanghaiDayAndFiltersUnsupportedItems(t *testing.T) {
	d := NewDiscoveryService()
	d.now = func() time.Time { return time.Date(2026, 10, 12, 18, 0, 0, 0, time.UTC) }
	var calls int
	d.client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return discoveryReply(`[
	{"weekday":{"id":1},"items":[{"id":1,"type":2,"name_cn":"周一作品","images":{}}]},
	{"weekday":{"id":2},"items":[{"id":2,"type":2,"name":"Original","name_cn":"周二新番","air_date":"2026-10-13","rating":{"score":8.4,"total":10},"images":{"large":"http://lain.bgm.tv/pic/cover/l/poster.jpg"}},{"id":3,"type":2,"nsfw":true,"name":"隐藏"},{"id":4,"type":1,"name":"书籍"}]}]`), nil
	})
	feed, err := d.Feed(context.Background(), "anime", "all", 1, 0)
	if err != nil || feed.Weekday != 2 || len(feed.Items) != 1 || feed.Items[0].Title != "周二新番" || !strings.HasPrefix(feed.Items[0].Poster, "https://") {
		t.Fatalf("calendar=%+v err=%v", feed, err)
	}
	other, err := d.Feed(context.Background(), "anime", "all", 1, 1)
	if err != nil || len(other.Items) != 1 || calls != 1 {
		t.Fatalf("weekday selection refetched entire calendar: %+v %d", other, calls)
	}
}

func TestDiscoveryCoalescesRequestsAndUsesMarkedStaleCache(t *testing.T) {
	d := NewDiscoveryService()
	var clock atomic.Int64
	clock.Store(time.Now().Unix())
	d.now = func() time.Time { return time.Unix(clock.Load(), 0) }
	var calls atomic.Int64
	var failing atomic.Bool
	d.client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if failing.Load() {
			return nil, errors.New("upstream unavailable")
		}
		time.Sleep(20 * time.Millisecond)
		return discoveryReply(`{"subjects":[{"id":"1","title":"保留内容","rate":"8.0"}]}`), nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := d.Feed(context.Background(), "movies", "all", 1, 0); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("upstream was requested %d times", calls.Load())
	}
	failing.Store(true)
	clock.Add(int64((31 * time.Minute).Seconds()))
	feed, err := d.Feed(context.Background(), "movies", "all", 1, 0)
	if err != nil || !feed.Stale || len(feed.Items) != 1 {
		t.Fatalf("stale feed: %+v %v", feed, err)
	}
	d.Feed(context.Background(), "movies", "all", 1, 0)
	if calls.Load() != 2 {
		t.Fatal("cooldown did not prevent repeated upstream failures")
	}
	clock.Add(int64((25 * time.Hour).Seconds()))
	if _, err = d.Feed(context.Background(), "movies", "all", 1, 0); err == nil {
		t.Fatal("expired cache was presented as usable")
	}
}

func TestDiscoveryPagingDoesNotMixCatalogPositions(t *testing.T) {
	d := NewDiscoveryService()
	d.client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if q.Get("type") != "tv" || q.Get("tag") != "日剧" || q.Get("page_start") != "24" {
			t.Fatalf("wrong paging: %s", r.URL)
		}
		return discoveryReply(`{"subjects":[]}`), nil
	})
	feed, err := d.Feed(context.Background(), "series", "jp", 2, 0)
	if err != nil || feed.Page != 2 || feed.HasMore || feed.Items == nil {
		t.Fatalf("paging: %+v %v", feed, err)
	}
}

func TestDiscoveryPosterCapabilitiesAndContentValidation(t *testing.T) {
	f := newManagementFixture(t, NewMemoryRepository())
	d := f.server.options.Discovery
	image, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aXioAAAAASUVORK5CYII=")
	var calls int
	d.client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) { calls++; return discoveryReply(string(image)), nil })
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		f.server.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	path := f.server.discoveryPosterURL("https://img3.doubanio.com/view/photo/s_ratio_poster/public/test.jpg")
	w := get(path)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("poster: %d %s", w.Code, w.Body.String())
	}
	get(path)
	if calls != 1 {
		t.Fatal("poster cache missed")
	}
	parsed, _ := url.Parse(path)
	q := parsed.Query()
	q.Set("s", "forged")
	parsed.RawQuery = q.Encode()
	if w = get(parsed.String()); w.Code != 403 {
		t.Fatal("forged capability allowed")
	}
	for _, raw := range []string{"https://127.0.0.1/private", "https://img3.doubanio.com@127.0.0.1/private", "https://img3.doubanio.com/other/path", "https://lain.bgm.tv/pic/cover/x?token=private"} {
		if normalizeDiscoveryPoster(raw) != "" {
			t.Fatalf("unsafe target accepted: %s", raw)
		}
	}
	q = url.Values{"u": {base64.RawURLEncoding.EncodeToString([]byte("https://127.0.0.1/private"))}, "e": {strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)}}
	q.Set("s", f.server.posterSignature(q.Get("u"), q.Get("e")))
	if w = get("/api/v1/discovery/poster?" + q.Encode()); w.Code != 403 {
		t.Fatal("signed private target accepted")
	}
	q = mustPosterQuery(t, path)
	q.Set("e", strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10))
	q.Set("s", f.server.posterSignature(q.Get("u"), q.Get("e")))
	if w = get("/api/v1/discovery/poster?" + q.Encode()); w.Code != 403 {
		t.Fatal("expired capability accepted")
	}
	d.client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) { return discoveryReply("<html>not an image</html>"), nil })
	if w = get(f.server.discoveryPosterURL("https://lain.bgm.tv/pic/cover/l/html.jpg")); w.Code != 502 {
		t.Fatal("non-image content was served")
	}
}

func mustPosterQuery(t *testing.T, path string) url.Values {
	t.Helper()
	u, err := url.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}
