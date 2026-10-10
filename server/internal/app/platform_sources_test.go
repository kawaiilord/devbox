package app

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func platformFixtureManager() (*MediaSourceManager, *atomic.Int64) {
	vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	manager := NewMediaSourceManager(NewMemoryRepository(), vault, false)
	calls := &atomic.Int64{}
	manager.platform = &platformResolver{run: func(_ context.Context, provider, target, cookie string, flat bool) (platformInfo, error) {
		calls.Add(1)
		info := platformInfo{Title: "Example video", ID: "abcDEF12345", WebpageURL: target, Duration: 30, Headers: map[string]string{"Cookie": "do-not-forward", "Authorization": "do-not-forward", "User-Agent": "Fixture"}}
		if flat {
			info.Type = "playlist"
			info.Entries = []platformInfo{{Title: "Episode one", WebpageURL: "https://www.youtube.com/watch?v=abcDEF12345"}, {Title: "Episode two", WebpageURL: "https://www.youtube.com/watch?v=abcDEF12346"}}
			return info, nil
		}
		info.Formats = []platformFormat{
			{ID: "360", URL: "https://cdn.example.test/video.mp4?signature=upstream-secret", Ext: "mp4", Protocol: "https", VideoCodec: "avc1.42c01e", AudioCodec: "mp4a.40.2", Height: 360, Bitrate: 800},
			{ID: "720", URL: "https://cdn.example.test/video-only.mp4?signature=upstream-secret", Ext: "mp4", Protocol: "https", VideoCodec: "avc1.42c01e", AudioCodec: "none", Height: 720, Bitrate: 1500},
			{ID: "audio", URL: "https://cdn.example.test/audio.m4a?signature=upstream-secret", Ext: "m4a", Protocol: "https", VideoCodec: "none", AudioCodec: "mp4a.40.2", Bitrate: 128},
			{ID: "hls", URL: "https://cdn.example.test/master.m3u8?signature=upstream-secret", Ext: "mp4", Protocol: "m3u8_native", VideoCodec: "avc1.42c01e", AudioCodec: "mp4a.40.2", Height: 480},
			{ID: "drm", URL: "https://cdn.example.test/drm.mp4", Ext: "mp4", Protocol: "https", VideoCodec: "avc1.42c01e", AudioCodec: "mp4a.40.2", HasDRM: true, Height: 2160},
		}
		return info, nil
	}}
	return manager, calls
}

func TestPlatformBindingsPlaylistsFormatsAndCredentialBoundaries(t *testing.T) {
	manager, calls := platformFixtureManager()
	ctx := context.Background()
	source, err := manager.SavePlatform(ctx, User{ID: "owner"}, "", "youtube", "YouTube", "SID=owner-secret")
	if err != nil {
		t.Fatal(err)
	}
	files, err := manager.ResolvePlatform(ctx, "owner", source.ID, "https://www.youtube.com/playlist?list=example")
	if err != nil || len(files) != 2 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	variants, err := manager.Variants(ctx, "owner", source.ID, files[0].Path)
	if err != nil || len(variants) != 4 {
		t.Fatalf("variants=%+v err=%v", variants, err)
	}
	ticket, err := manager.PrepareMediaVariant(ctx, "owner", source.ID, files[0].Path, "720")
	if err != nil || ticket.Kind != "platform-dash" || ticket.AudioURL == "" {
		t.Fatalf("DASH ticket=%+v err=%v", ticket, err)
	}
	if ticket.StreamHeaders["Cookie"] != "" || ticket.StreamHeaders["Authorization"] != "" {
		t.Fatal("extractor credentials were copied into a ticket")
	}
	if _, err = manager.PrepareMediaVariant(ctx, "owner", source.ID, files[0].Path, "drm"); err == nil {
		t.Fatal("DRM format accepted")
	}
	if _, err = manager.ResolvePlatform(ctx, "stranger", source.ID, "https://youtu.be/abcDEF12345"); err == nil {
		t.Fatal("source access crossed ownership")
	}
	manager.platformClient = &http.Client{Transport: quarkTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials sent to a third-party CDN")
		}
		if r.Header.Get("Range") != "bytes=1-4" {
			t.Error("Range missing")
		}
		w := httptest.NewRecorder()
		w.WriteHeader(206)
		_, _ = w.Write([]byte("data"))
		response := w.Result()
		response.Request = r
		return response, nil
	})}
	response, err := manager.Open(ctx, ticket, "GET", "bytes=1-4", "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "data" {
		t.Fatal("platform range playback failed")
	}
	if calls.Load() != 2 {
		t.Fatalf("metadata was not cached: %d calls", calls.Load())
	}
	before := calls.Load()
	_, err = manager.SavePlatform(ctx, User{ID: "owner"}, source.ID, "youtube", "YouTube", "SID=new-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Variants(ctx, "owner", source.ID, files[0].Path); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != before+1 {
		t.Fatal("reauthentication reused old account metadata")
	}
	raw, _ := json.Marshal(source)
	if strings.Contains(string(raw), "owner-secret") {
		t.Fatal("provider binding returned cookie")
	}
	for _, bad := range []string{"https://youtube.com.evil.test/watch?v=abcDEF12345", "https://www.youtube.com:9000/watch?v=abcDEF12345", "https://user:pass@www.youtube.com/watch?v=abcDEF12345", "file:///etc/passwd"} {
		if _, err = canonicalPlatformURL("youtube", bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestPlatformHLSResourcesAreEncryptedAndBoundToTicket(t *testing.T) {
	manager, _ := platformFixtureManager()
	server := NewServer(Options{Sources: manager})
	raw := "opaque-parent-ticket"
	manifest := []byte("#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,URI=\"audio/index.m3u8?signature=private\"\n#EXT-X-KEY:METHOD=AES-128,URI=\"../key?signature=private\"\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\nvideo/index.m3u8?signature=private\n")
	output, err := server.rewriteHLS(raw, "https://cdn.example.test/hls/master.m3u8", manifest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), "cdn.example.test") || strings.Contains(string(output), "signature=private") {
		t.Fatal("HLS exposed upstream signed URLs")
	}
	lines := strings.Split(string(output), "\n")
	resource := lines[len(lines)-2]
	request := httptest.NewRequest("GET", resource, nil)
	encrypted := request.URL.Query().Get("asset")
	asset, err := server.decodePlatformAsset(raw, encrypted)
	if err != nil || asset.URL != "https://cdn.example.test/hls/video/index.m3u8?signature=private" || !asset.Manifest {
		t.Fatalf("asset=%+v err=%v", asset, err)
	}
	if _, err = server.decodePlatformAsset("different-parent", encrypted); err == nil {
		t.Fatal("child resource escaped its parent ticket")
	}
	if _, err = server.decodePlatformAsset(raw, encrypted+"x"); err == nil {
		t.Fatal("modified child resource accepted")
	}
	if _, err = server.rewriteHLS(raw, "https://cdn.example.test/master.m3u8", []byte("#EXTM3U\nhttp://127.0.0.1/private\n")); err == nil {
		t.Fatal("manifest accepted local network target")
	}
}

func TestPlatformDASHUsesBoundedMP4IndexesAndOpaqueSegmentURLs(t *testing.T) {
	manager, _ := platformFixtureManager()
	ctx := context.Background()
	source, err := manager.SavePlatform(ctx, User{ID: "owner"}, "", "youtube", "YT", "")
	if err != nil {
		t.Fatal(err)
	}
	box := func(kind string, n int) []byte {
		b := make([]byte, n)
		binary.BigEndian.PutUint32(b, uint32(n))
		copy(b[4:8], kind)
		return b
	}
	media := append(box("ftyp", 16), box("moov", 24)...)
	media = append(media, box("sidx", 32)...)
	media = append(media, box("mdat", 16)...)
	manager.platformClient = &http.Client{Transport: quarkTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") != "bytes=0-1048575" {
			t.Error("MP4 index probe was not bounded")
		}
		w := httptest.NewRecorder()
		w.WriteHeader(206)
		_, _ = w.Write(media)
		response := w.Result()
		response.Request = r
		return response, nil
	})}
	ticket, err := manager.PrepareMediaVariant(ctx, "owner", source.ID, platformMediaPath("https://www.youtube.com/watch?v=abcDEF12345"), "720")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Sources: manager})
	mpd, err := server.platformMPD(ctx, "parent-ticket", ticket)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mpd), "upstream-secret") || strings.Contains(string(mpd), "cdn.example.test") || !strings.Contains(string(mpd), `indexRange="40-71"`) || !strings.Contains(string(mpd), `range="0-39"`) {
		t.Fatalf("invalid generated MPD: %s", mpd)
	}
	if _, err = readMP4Index([]byte("not an mp4")); err == nil {
		t.Fatal("invalid MP4 index accepted")
	}
}

func TestExtractorProcessUsesPrivateTemporaryCookiesAndNoShellInterpolation(t *testing.T) {
	dir := t.TempDir()
	program := filepath.Join(dir, "extractor")
	argsFile := filepath.Join(dir, "args")
	cookiePathFile := filepath.Join(dir, "cookie-path")
	t.Setenv("EXTRACTOR_ARGS_FILE", argsFile)
	t.Setenv("EXTRACTOR_COOKIE_PATH_FILE", cookiePathFile)
	script := `#!/bin/sh
printf '%s\n' "$@" > "$EXTRACTOR_ARGS_FILE"
previous=''
for value in "$@"; do
 if [ "$previous" = '--cookies' ]; then
  [ "$(stat -c %a "$value")" = '600' ] || exit 12
  printf '%s' "$value" > "$EXTRACTOR_COOKIE_PATH_FILE"
 fi
 previous="$value"
done
printf '{"id":"video","title":"Test","formats":[]}'
`
	if err := os.WriteFile(program, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	r := platformResolver{path: program}
	_, err := r.execute(context.Background(), "youtube", "https://www.youtube.com/watch?v=abcDEF12345&literal=$(false)", "SID=private-cookie", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(argsFile)
	if strings.Contains(string(args), "private-cookie") || !strings.Contains(string(args), "--ignore-config") || !strings.Contains(string(args), "--no-plugin-dirs") || !strings.Contains(string(args), "--\nhttps://") {
		t.Fatal("unsafe extractor arguments")
	}
	cookiePath, _ := os.ReadFile(cookiePathFile)
	if _, err = os.Stat(string(cookiePath)); !os.IsNotExist(err) {
		t.Fatal("temporary credentials were retained")
	}
}

func TestPlatformSingleFormatFallback(t *testing.T) {
	formats := supportedPlatformFormats(platformInfo{URL: "https://cdn.example.test/movie.mp4", Ext: "mp4", Protocol: "https"})
	if len(formats) != 1 || formats[0].ID != "default" {
		t.Fatal("single-format platform result was discarded")
	}
}
