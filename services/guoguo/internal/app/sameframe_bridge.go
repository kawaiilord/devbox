package app

// The SameFrame bridge exposes only provider data and shared, stateless media.
// Account state, downloads and the original unauthenticated UI are not mounted.
import (
	"bytes"
	"context"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type bridgePlaylist struct {
	session string
	body    []byte
}
type bridgeRecorder struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (w *bridgeRecorder) Header() http.Header { return w.header }
func (w *bridgeRecorder) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
}
func (w *bridgeRecorder) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = 200
	}
	return w.body.Write(b)
}

func (a *UIApp) sameframeRoutes(token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/bridge/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ready": true}) })
	mux.HandleFunc("/bridge/recommendations", a.handleRecommendations)
	mux.HandleFunc("/bridge/rankings", a.handleRankings)
	mux.HandleFunc("/bridge/search", a.handleLibrarySearch)
	mux.HandleFunc("/bridge/detail", a.bridgeDetail)
	mux.HandleFunc("/bridge/image", a.handleImage)
	mux.HandleFunc("/bridge/segment.ts", a.handleEmbySegment)
	// An episode's conversion is shared by room participants. Fetching a manifest
	// must not create a separate FFmpeg session for every viewer or HLS retry.
	var mu sync.Mutex
	playlists := map[string]bridgePlaylist{}
	mux.HandleFunc("/bridge/stream.m3u8", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(405)
			return
		}
		id, episode := r.URL.Query().Get("id"), r.URL.Query().Get("episode")
		if !hongguoNumericID.MatchString(id) || !hongguoNumericID.MatchString(episode) {
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		a.playbackMu.Lock()
		for key, entry := range playlists {
			if a.playbacks[entry.session] == nil {
				delete(playlists, key)
			}
		}
		entry, ok := playlists[id+":"+episode]
		if ok {
			a.touchPlaybackLocked(a.playbacks[entry.session])
		}
		a.playbackMu.Unlock()
		if !ok {
			if r.Method == http.MethodHead {
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
				return
			}
			key, err := a.embySigningKey(true)
			if err != nil {
				writeJSON(w, 503, map[string]string{"error": "短剧播放服务未就绪"})
				return
			}
			dramaID, chapter := providerDramaID(sourceHongguo, id), providerChapterID(sourceHongguo, id, episode)
			req := r.Clone(r.Context())
			u := *r.URL
			req.URL = &u
			req.URL.RawQuery = url.Values{"id": {dramaID}, "chapter": {chapter}, "key": {embyToken(key, dramaID, chapter)}}.Encode()
			recorder := &bridgeRecorder{header: make(http.Header)}
			a.handleEmbyStream(recorder, req)
			if recorder.code != 200 {
				writeJSON(w, 502, map[string]string{"error": "短剧播放暂不可用，请稍后重试"})
				return
			}
			entry.body = append([]byte(nil), recorder.body.Bytes()...)
			for _, line := range strings.Split(string(entry.body), "\n") {
				if strings.HasPrefix(line, "segment.ts?") {
					segment, _ := url.Parse(line)
					entry.session = segment.Query().Get("session")
					break
				}
			}
			if entry.session == "" {
				w.WriteHeader(502)
				return
			}
			playlists[id+":"+episode] = entry
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodHead {
			_, _ = w.Write(entry.body)
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

func (a *UIApp) bridgeDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	id := r.URL.Query().Get("id")
	if !hongguoNumericID.MatchString(id) {
		w.WriteHeader(400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	entry, err := a.downloader.hongguoAppDetail(ctx, id)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "短剧详情暂不可用，请稍后重试"})
		return
	}
	// Only stable episode identifiers leave the provider, never expiring media
	// URLs, content keys or app signing material.
	episodes := []map[string]string{}
	for i, ch := range entry.Chapters {
		parts := strings.Split(ch.ID, ":")
		video := parts[len(parts)-1]
		if hongguoNumericID.MatchString(video) {
			episodes = append(episodes, map[string]string{"id": video, "name": "第" + ch.EpisodeString(i+1) + "集"})
		}
	}
	writeJSON(w, 200, map[string]any{"drama": entry.Drama, "episodes": episodes})
}

// Compile-time assertion keeps the buffered manifest response faithful to net/http.
var _ http.ResponseWriter = (*bridgeRecorder)(nil)
