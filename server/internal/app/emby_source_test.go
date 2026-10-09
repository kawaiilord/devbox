package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestEmbyCredentialsBrowsingPlaybackSubtitlesAndLogout(t *testing.T) {
	const (
		embyUserID = "emby-user-1"
		embyToken  = "emby-access-token"
	)
	var logoutCalled atomic.Bool
	itemResponse := map[string]any{
		"Id": "movie1", "Name": "Movie One", "Type": "Movie", "MediaType": "Video",
		"Container": "mkv",
		"MediaSources": []map[string]any{{
			"Id": "media-source-1", "Container": "mkv", "Size": 10,
			"SupportsDirectPlay": true, "SupportsDirectStream": true,
			"MediaStreams": []map[string]any{{
				"Index": 2, "Type": "Subtitle", "Codec": "srt", "Language": "zh",
				"DisplayTitle": "中文", "IsTextSubtitleStream": true,
			}},
		}},
	}
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/emby/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/emby/Users/AuthenticateByName" {
			if r.Method != http.MethodPost || r.Header.Get("X-Emby-Token") != "" {
				http.Error(w, "bad authentication request", http.StatusBadRequest)
				return
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"Username":"viewer"`) ||
				!strings.Contains(string(body), `"Pw":"top-secret"`) ||
				strings.Contains(r.Header.Get("X-Emby-Authorization"), "top-secret") {
				http.Error(w, "credentials", http.StatusUnauthorized)
				return
			}
			writeTestJSON(w, map[string]any{
				"AccessToken": embyToken, "ServerId": "server-1",
				"User": map[string]string{"Id": embyUserID},
			})
			return
		}
		if r.Header.Get("X-Emby-Token") != embyToken ||
			!strings.Contains(r.Header.Get("X-Emby-Authorization"), `Token="`+embyToken+`"`) ||
			r.URL.Query().Get("api_key") != "" {
			http.Error(w, "missing token header", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/emby/Users/" + embyUserID + "/Items":
			if r.URL.Query().Get("ParentId") == "" {
				writeTestJSON(w, map[string]any{"Items": []map[string]any{{
					"Id": "library1", "Name": "Movies", "Type": "CollectionFolder", "IsFolder": true,
				}}, "TotalRecordCount": 1})
				return
			}
			if r.URL.Query().Get("ParentId") != "library1" {
				http.Error(w, "parent", http.StatusBadRequest)
				return
			}
			writeTestJSON(w, map[string]any{
				"Items": []any{
					itemResponse,
					map[string]any{"Id": "audio1", "Name": "Audio", "Type": "Audio", "MediaType": "Audio"},
				},
				"TotalRecordCount": 2,
			})
		case "/emby/Users/" + embyUserID + "/Items/movie1":
			writeTestJSON(w, itemResponse)
		case "/emby/Videos/movie1/stream.mkv":
			if r.URL.Query().Get("Static") != "true" ||
				r.URL.Query().Get("MediaSourceId") != "media-source-1" ||
				r.URL.Query().Get("PlaySessionId") == "" ||
				r.Header.Get("Range") != "bytes=0-3" {
				http.Error(w, "stream parameters", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "video/x-matroska")
			w.Header().Set("Content-Range", "bytes 0-3/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "data")
		case "/emby/Videos/movie1/media-source-1/Subtitles/2/Stream.vtt":
			w.Header().Set("Content-Type", "text/vtt")
			_, _ = io.WriteString(w, "WEBVTT\n")
		case "/emby/Sessions/Logout":
			logoutCalled.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer emby.Close()

	vaultKey := make([]byte, 32)
	for index := range vaultKey {
		vaultKey[index] = byte(index + 7)
	}
	vault, err := NewCredentialVault(base64.StdEncoding.EncodeToString(vaultKey))
	if err != nil {
		t.Fatal(err)
	}
	repository := NewMemoryRepository()
	manager := NewMediaSourceManager(repository, vault, true)
	ctx := context.Background()
	user := User{ID: "sameframe-user", DisplayName: "Viewer"}
	source, err := manager.CreateEmby(
		ctx, user, "Home Emby", emby.URL+"/emby/", "viewer", "top-secret",
	)
	if err != nil {
		t.Fatal(err)
	}
	if source.Type != "emby" {
		t.Fatalf("source=%+v", source)
	}
	stored, err := repository.GetMediaSource(ctx, user.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.CredentialsCiphertext, embyToken) ||
		strings.Contains(stored.CredentialsCiphertext, "top-secret") {
		t.Fatalf("Emby secret leaked into storage: %q", stored.CredentialsCiphertext)
	}
	root, err := manager.Browse(ctx, user.ID, source.ID, "/")
	if err != nil || len(root) != 1 || !root[0].IsDirectory || root[0].Path != "/library1" {
		t.Fatalf("root=%+v error=%v", root, err)
	}
	files, err := manager.Browse(ctx, user.ID, source.ID, "/library1/")
	if err != nil || len(files) != 1 || files[0].Path != "/library1/movie1" || files[0].Size != 10 {
		t.Fatalf("files=%+v error=%v", files, err)
	}
	ticket, err := manager.PrepareMediaTicket(ctx, user.ID, source.ID, files[0].Path)
	if err != nil || ticket.Kind != "emby-video" || ticket.ItemID != "movie1" || ticket.MediaSourceID != "media-source-1" {
		t.Fatalf("ticket=%+v error=%v", ticket, err)
	}
	response, err := manager.Open(ctx, ticket, http.MethodGet, "bytes=0-3", "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusPartialContent || string(body) != "data" {
		t.Fatalf("status=%d body=%q", response.StatusCode, body)
	}
	subtitles, err := manager.Subtitles(ctx, user.ID, source.ID, files[0].Path)
	if err != nil || len(subtitles) != 1 || subtitles[0].Name != "中文" {
		t.Fatalf("subtitles=%+v error=%v", subtitles, err)
	}
	subtitleTicket, err := manager.PrepareSubtitleTicket(
		ctx, user.ID, source.ID, files[0].Path, subtitles[0].Path,
	)
	if err != nil || subtitleTicket.Kind != "emby-subtitle" || subtitleTicket.SubtitleIndex != 2 {
		t.Fatalf("subtitle ticket=%+v error=%v", subtitleTicket, err)
	}
	subtitleResponse, err := manager.Open(ctx, subtitleTicket, http.MethodGet, "", "")
	if err != nil {
		t.Fatal(err)
	}
	subtitleBody, _ := io.ReadAll(subtitleResponse.Body)
	subtitleResponse.Body.Close()
	if string(subtitleBody) != "WEBVTT\n" {
		t.Fatalf("subtitle body=%q", subtitleBody)
	}
	if err := manager.Delete(ctx, user.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	if !logoutCalled.Load() {
		t.Fatal("Emby session was not logged out on source deletion")
	}
}

func TestEmbyRejectsInvalidVirtualPaths(t *testing.T) {
	for _, value := range []string{"/../secret", "/folder/%2fitem", "/folder/@subtitle", "/folder/.."} {
		if _, err := embyItemIDFromPath(value, false); err == nil {
			t.Fatalf("path %q was accepted", value)
		}
	}
}

func TestEmbyCredentialsAreNotForwardedAcrossPortRedirects(t *testing.T) {
	var targetHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/stolen", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	manager := NewMediaSourceManager(NewMemoryRepository(), vault, true)
	_, err := manager.CreateEmby(
		context.Background(), User{ID: "user"}, "Redirect", redirector.URL+"/emby/", "viewer", "secret",
	)
	if err == nil {
		t.Fatal("cross-port Emby redirect was accepted")
	}
	if targetHits.Load() != 0 {
		t.Fatal("Emby credentials were sent to a redirected port")
	}
}

func writeTestJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
