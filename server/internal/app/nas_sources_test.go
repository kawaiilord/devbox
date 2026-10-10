package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNASNativeLoginBrowseAndRangePlayback(t *testing.T) {
	for _, provider := range []string{"synology", "qnap", "nextcloud", "seafile", "truenas"} {
		t.Run(provider, func(t *testing.T) {
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				stream := func() {
					if r.Header.Get("Range") != "bytes=1-4" {
						t.Error("stream lost Range")
					}
					w.Header().Set("Content-Type", "video/mp4")
					w.Header().Set("Content-Range", "bytes 1-4/10")
					w.WriteHeader(206)
					_, _ = io.WriteString(w, "data")
				}
				switch r.URL.Path {
				case "/webapi/query.cgi":
					writeTestJSON(w, map[string]any{"success": true, "data": map[string]any{"SYNO.API.Auth": map[string]any{"path": "auth.cgi", "maxVersion": 7}, "SYNO.FileStation.List": map[string]any{"path": "entry.cgi", "maxVersion": 2}, "SYNO.FileStation.Download": map[string]any{"path": "entry.cgi", "maxVersion": 2}}})
				case "/webapi/auth.cgi":
					if r.Form.Get("account") != "viewer" || r.Form.Get("passwd") != "top-secret" {
						t.Error("Synology login body wrong")
					}
					writeTestJSON(w, map[string]any{"success": true, "data": map[string]string{"sid": "sid-secret"}})
				case "/webapi/entry.cgi":
					if r.Form.Get("_sid") != "sid-secret" {
						t.Error("Synology session missing")
					}
					if r.Form.Get("method") == "download" {
						stream()
						return
					}
					if r.Form.Get("method") == "list_share" {
						writeTestJSON(w, map[string]any{"success": true, "data": map[string]any{"total": 1, "shares": []any{map[string]any{"name": "Movies", "path": "/Movies", "isdir": true}}}})
					} else {
						writeTestJSON(w, map[string]any{"success": true, "data": map[string]any{"total": 1, "files": []any{map[string]any{"name": "movie.mp4", "path": "/Movies/movie.mp4", "isdir": false, "additional": map[string]int{"size": 10}}}}})
					}
				case "/cgi-bin/filemanager/wfm2Login.cgi":
					if r.Form.Get("user") != "viewer" || r.Form.Get("pwd") != base64.StdEncoding.EncodeToString([]byte("top-secret")) {
						t.Error("QNAP login wrong")
					}
					writeTestJSON(w, map[string]any{"status": 1, "sid": "sid-secret"})
				case "/cgi-bin/filemanager/utilRequest.cgi":
					if r.Form.Get("sid") != "sid-secret" {
						t.Error("QNAP session missing")
					}
					switch r.Form.Get("func") {
					case "download":
						stream()
					case "get_tree":
						writeTestJSON(w, []any{map[string]string{"id": "/Movies", "text": "Movies"}})
					default:
						writeTestJSON(w, map[string]any{"status": "1", "total": 1, "datas": []any{map[string]any{"filename": "movie.mp4", "isfolder": "0", "filesize": "10"}}})
					}
				case "/ocs/v2.php/cloud/user":
					username, password, _ := r.BasicAuth()
					if username != "viewer" || password != "top-secret" || r.Header.Get("OCS-APIRequest") != "true" {
						t.Error("Nextcloud auth wrong")
					}
					writeTestJSON(w, map[string]any{"ocs": map[string]any{"meta": map[string]int{"statuscode": 100}, "data": map[string]string{"id": "viewer"}}})
				case "/remote.php/dav/files/viewer/", "/remote.php/dav/files/viewer/movie.mp4":
					username, password, _ := r.BasicAuth()
					if username != "viewer" || password != "top-secret" {
						t.Error("DAV auth wrong")
					}
					if r.Method == "PROPFIND" {
						w.Header().Set("Content-Type", "application/xml")
						w.WriteHeader(207)
						_, _ = io.WriteString(w, `<D:multistatus xmlns:D="DAV:"><D:response><D:href>/remote.php/dav/files/viewer/movie.mp4</D:href><D:propstat><D:status>HTTP/1.1 200 OK</D:status><D:prop><D:displayname>movie.mp4</D:displayname><D:getcontentlength>10</D:getcontentlength><D:resourcetype/></D:prop></D:propstat></D:response></D:multistatus>`)
					} else {
						stream()
					}
				case "/api2/auth-token/":
					if r.Form.Get("username") != "viewer" || r.Form.Get("password") != "top-secret" {
						t.Error("Seafile login wrong")
					}
					writeTestJSON(w, map[string]string{"token": "session-secret"})
				case "/api2/repos/":
					if r.Header.Get("Authorization") != "Token session-secret" {
						t.Error("Seafile token missing")
					}
					writeTestJSON(w, []any{map[string]any{"id": "library-1", "name": "Movies", "encrypted": false}})
				case "/api/v2.1/repos/library-1/dir/":
					writeTestJSON(w, map[string]any{"dirent_list": []any{map[string]any{"name": "movie.mp4", "type": "file", "size": 10}}})
				case "/api2/repos/library-1/file/":
					writeTestJSON(w, upstream.URL+"/download?token=temporary-secret")
				case "/api/v2.0/system/info/":
					if r.Header.Get("Authorization") != "Bearer api-secret" {
						t.Error("TrueNAS token missing")
					}
					writeTestJSON(w, map[string]string{"hostname": "NAS"})
				case "/api/v2.0/filesystem/listdir/":
					if r.URL.Query().Get("path") != "/mnt" {
						t.Error("TrueNAS escaped storage root")
					}
					writeTestJSON(w, []any{map[string]any{"name": "movie.mp4", "type": "FILE", "size": 10}})
				case "/api/v2.0/filesystem/stat/":
					writeTestJSON(w, map[string]string{"realpath": "/mnt/movie.mp4"})
				case "/api/v2.0/core/download/":
					writeTestJSON(w, []any{1, "/download?token=temporary-secret"})
				case "/download":
					if r.Header.Get("Authorization") != "" {
						t.Error("signed download received account token")
					}
					stream()
				default:
					t.Errorf("unexpected NAS route %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			repo := NewMemoryRepository()
			vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
			manager := NewMediaSourceManager(repo, vault, true)
			user := User{ID: "nas-owner"}
			source, err := manager.SaveNAS(context.Background(), user, "", NASSourceInput{Provider: provider, Name: "Home NAS", BaseURL: upstream.URL, Username: "viewer", Password: "top-secret", Token: "api-secret"})
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(source)
			for _, secret := range []string{"top-secret", "sid-secret", "session-secret", "api-secret"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatal("source response exposed credentials")
				}
			}
			files, err := manager.Browse(context.Background(), user.ID, source.ID, "/")
			if err != nil || len(files) != 1 {
				t.Fatalf("root=%+v err=%v", files, err)
			}
			if files[0].IsDirectory {
				files, err = manager.Browse(context.Background(), user.ID, source.ID, files[0].Path)
				if err != nil {
					t.Fatal(err)
				}
			}
			if len(files) != 1 || files[0].Name != "movie.mp4" {
				t.Fatalf("files=%+v", files)
			}
			ticket, err := manager.PrepareMediaTicket(context.Background(), user.ID, source.ID, files[0].Path)
			if err != nil {
				t.Fatal(err)
			}
			response, err := manager.Open(context.Background(), ticket, http.MethodGet, "bytes=1-4", "")
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 206 || string(body) != "data" {
				t.Fatalf("playback=%d %q", response.StatusCode, body)
			}
			if _, err = manager.Browse(context.Background(), "stranger", source.ID, "/"); err == nil {
				t.Fatal("another user browsed NAS")
			}
			if _, err = manager.PrepareMediaTicket(context.Background(), user.ID, source.ID, "/../secret.mp4"); err == nil {
				t.Fatal("NAS accepted path traversal")
			}
		})
	}
}

func TestNASPrivateAddressRequiresExactConfiguredAuthority(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(207)
		_, _ = io.WriteString(w, `<D:multistatus xmlns:D="DAV:"/>`)
	}))
	defer upstream.Close()
	t.Setenv("SAMEFRAME_SOURCE_HOST_ALLOWLIST", strings.TrimPrefix(upstream.URL, "http://"))
	vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	manager := NewMediaSourceManager(NewMemoryRepository(), vault, false)
	source, err := manager.CreateWebDAV(context.Background(), User{ID: "owner"}, "LAN NAS", upstream.URL, "viewer", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Browse(context.Background(), "owner", source.ID, "/"); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.CreateWebDAV(context.Background(), User{ID: "owner"}, "Other private service", "https://127.0.0.1:1/", "viewer", "secret"); err == nil {
		t.Fatal("host allowlist permitted a different private service port")
	}
}
