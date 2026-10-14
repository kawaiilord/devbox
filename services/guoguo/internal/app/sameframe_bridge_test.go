package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
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
