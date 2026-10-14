package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCatalogAPTVChannelMetadataAndUserAgent(t *testing.T) {
	content := "#EXTM3U\n#EXT-X-APP APTV\n#EXT-X-APTV-TYPE remote\n#EXT-X-SUB-URL https://feed.example/list.m3u\n#EXTINF:-1 tvg-id=\"one\" http-user-agent=\"AptvPlayer-UA\",Channel One\nhttps://media.example/one.m3u8\n#EXTINF:-1,Channel Two\n#EXTVLCOPT:http-user-agent=OtherPlayer\nhttps://media.example/two.m3u8\n#EXTINF:-1,Channel Three\nhttps://media.example/three.m3u8\n"
	channels, skipped, err := parseCatalogM3U([]byte(content), "https://feed.example/list.m3u")
	if err != nil || skipped != 0 || len(channels) != 3 {
		t.Fatalf("channels=%+v skipped=%d error=%v", channels, skipped, err)
	}
	if channels[0].UserAgent != "AptvPlayer-UA" || channels[1].UserAgent != "OtherPlayer" || channels[2].UserAgent != "" {
		t.Fatal("channel User-Agent is lost or leaks to the next entry")
	}
	for _, tag := range []string{"#EXT-X-TARGETDURATION:6", "#EXT-X-STREAM-INF:BANDWIDTH=800000", "#EXT-X-MEDIA-SEQUENCE:10", "#EXT-X-KEY:METHOD=AES-128,URI=\"key\""} {
		if _, _, err := parseCatalogM3U([]byte(content+tag+"\n"), ""); err == nil {
			t.Fatalf("HLS tag accepted as channel metadata: %s", tag)
		}
	}
	bad := "#EXTM3U\n#EXTINF:-1 http-user-agent=\"bad\rInjected: value\",Unsafe\nhttps://media.example/bad.m3u8\n"
	if _, _, err := parseCatalogM3U([]byte(bad), ""); err == nil {
		t.Fatal("header control characters accepted")
	}
	legacy := "#EXTM3U\n#EXTINF:-1,Same\nhttps://media.example/a.m3u8\n#EXTINF:-1,Same\nhttps://media.example/b.m3u8\n"
	old, _, err := parseCatalogM3U([]byte(legacy), "")
	if err != nil || old[1].ID != catalogHash(":Same:https://media.example/b.m3u8") {
		t.Fatal("legacy channel ID changed")
	}
}

func TestCatalogAPTVAgentSurvivesProbeTicketAndAssetFetch(t *testing.T) {
	f, c := catalogFixture(t)
	ctx := context.Background()
	content := "#EXTM3U\n#EXT-X-APP APTV\n#EXTINF:-1 http-user-agent=\"AptvPlayer-UA\",Test\nhttps://media.example/live.m3u8\n"
	token, _, err := c.preview(ctx, f.root.User.ID, catalogPreviewInput{Name: "APTV", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.commit(ctx, f.root.User.ID, token); err != nil {
		t.Fatal(err)
	}
	state, _, err := c.read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	src := catalogSources(state, true)[0]
	if src.Channels[0].UserAgent != "AptvPlayer-UA" {
		t.Fatal("agent not persisted in encrypted subscription")
	}
	c.client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("User-Agent") != "AptvPlayer-UA" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("unexpected upstream headers: %v", r.Header)
		}
		return discoveryReply("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nsegment.ts\n"), nil
	})
	film := channelFilm(src, src.Channels[0])
	encoded, _ := json.Marshal(film)
	if strings.Contains(string(encoded), "AptvPlayer-UA") {
		t.Fatal("upstream-only header leaked into film metadata")
	}
	bound, err := c.bind(ctx, f.member.User, src)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := f.server.sources.PrepareMediaTicket(ctx, f.member.User.ID, bound.ID, catalogPath(film.ID, "live", "live"))
	if err != nil {
		t.Fatal(err)
	}
	if ticket.StreamHeaders["User-Agent"] != "AptvPlayer-UA" {
		t.Fatal("ticket lost user agent")
	}
	ticket.StreamURL = "https://media.example/segment.ts"
	ticket.Kind = "platform-asset"
	r, err := f.server.sources.Open(ctx, ticket, "GET", "bytes=0-31", "")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, r.Body)
	r.Body.Close()
	if _, err := c.mediaRequest(ctx, "https://media.example/x", "GET", "", "", "bad\r\nHost: secret"); err == nil {
		t.Fatal("request accepted invalid user agent")
	}
}
