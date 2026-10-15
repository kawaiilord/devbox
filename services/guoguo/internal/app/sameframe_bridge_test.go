package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSameFrameBridgeAuthAndRouteIsolation(t *testing.T) {
	app := &UIApp{}
	token := strings.Repeat("secret", 8)
	handler := app.sameframeRoutes(token)
	for _, path := range []string{"/bridge/health", "/bridge/detail?id=1", "/api/ui/config"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Fatal("anonymous provider request accepted", path, w.Code)
		}
	}
	for _, path := range []string{"/", "/api/ui/config", "/api/ui/download", "/api/ui/following", "/api/ui/playback/history", "/assets/main.js"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatal("unwanted route mounted", path, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/bridge/health", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("health", w.Code)
	}
	w = httptest.NewRecorder()
	app.sameframeRoutes("").ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("empty service secret accepted")
	}
}

func TestSameFrameBridgeSharesEpisodeAndServesRealHLS(t *testing.T) {
	app, _ := nativePlaybackFixture(t, "4", "320x180")
	token := strings.Repeat("secret", 8)
	server := httptest.NewServer(app.sameframeRoutes(token))
	defer server.Close()
	defer app.closePlaybacks()
	fetch := func(path string) (int, []byte) {
		r, _ := http.NewRequest("GET", server.URL+path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		res, e := server.Client().Do(r)
		if e != nil {
			t.Error(e)
			return 0, nil
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		return res.StatusCode, data
	}
	manifests := make([][]byte, 6)
	var wg sync.WaitGroup
	for i := range manifests {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			status, data := fetch("/bridge/stream.m3u8?id=7000000000000000001&episode=1")
			if status != 200 {
				t.Errorf("viewer %d status %d: %s", index, status, data)
			}
			manifests[index] = data
		}(i)
	}
	wg.Wait()
	for _, m := range manifests {
		if string(m) != string(manifests[0]) || !strings.Contains(string(m), "#EXT-X-ENDLIST") {
			t.Fatal("viewers do not share a complete manifest")
		}
	}
	count := 0
	app.playbackMu.Lock()
	for _, s := range app.playbacks {
		if s.native != nil {
			count++
		}
	}
	app.playbackMu.Unlock()
	if count != 1 {
		t.Fatal("expected one conversion for six viewers", count)
	}
	for _, line := range strings.Split(string(manifests[0]), "\n") {
		if strings.HasPrefix(line, "segment.ts?") {
			status, data := fetch("/bridge/" + line)
			if status != 200 || len(data) < 188 || data[0] != 0x47 {
				t.Fatal("TS media not served", status, len(data))
			}
			break
		}
	}
}

func TestSameFrameWebDetailFallbackKeepsStableEpisodeIDs(t *testing.T) {
	var calls atomic.Int32
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		body := ""
		if r.URL.Path == "/detail" {
			body = `<script>_ROUTER_DATA={"loaderData":{"detail_page":{"seriesDetail":{"series_name":"网页回退样本","vid_list":["11","22"]}}}}</script>`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	title, chapters, err := d.fetchHongguoChapters(ctx, "7000000000000000001")
	if err != nil || title != "网页回退样本" || len(chapters) != 2 || chapters[1].ID != "hongguo:7000000000000000001:22" {
		t.Fatal(title, chapters, err)
	}
	before := calls.Load()
	_, again, err := d.fetchHongguoChapters(ctx, "7000000000000000001")
	if err != nil || len(again) != 2 || calls.Load() != before {
		t.Fatal("web detail fallback was not cached", err)
	}
}

func TestSameFrameRankingUsesDesktopDocument(t *testing.T) {
	board := rankingBoards[0]
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("User-Agent") != rankingWebUserAgent {
			t.Error("mobile rank document may omit streamed content")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(rankingFixture(board, 1, rankingRows(1)))), Request: r}, nil
	})
	page, err := d.fetchRankingPage(context.Background(), board, 1)
	if err != nil || len(page.Items) != 2 {
		t.Fatal(page, err)
	}
}

func TestSameFrameSharedEpisodeServesDifferentPositionsWithoutPreemption(t *testing.T) {
	app, fixture := nativePlaybackFixture(t, "80", "320x180")
	cache := newPlaybackNative(app, context.Background(), fixture.tasks[0], fixture.downloadIDs[0], 0, 0, false)
	cache.shared = true
	defer cache.Close()
	cache.start()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, part := range []int{0, 19, 33, 2, 20, 34} {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			body, err := cache.segment(ctx, index)
			if err != nil || len(body) < 188 || body[0] != 0x47 {
				t.Errorf("segment %d: %v bytes %d", index, err, len(body))
			}
		}(part)
	}
	wg.Wait()
}

func TestSharedPlaybackPrefillsNextBatchWithTwelveSecondsRemaining(t *testing.T) {
	app, fixture := nativePlaybackFixture(t, "60", "320x180")
	cache := newPlaybackNative(app, context.Background(), fixture.tasks[0], fixture.downloadIDs[0], 0, 0, false)
	cache.shared = true
	defer cache.Close()
	cache.start()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := cache.segment(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.segment(ctx, 6); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cache.mu.Lock()
		started := cache.job != nil && cache.job.start >= playbackNativeBatchSegments
		cache.mu.Unlock()
		if started {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("next batch was not prepared while twelve seconds remained")
}
