package app

import (
	"context"
	"encoding/json"
	apb "github.com/kawaiilord/devbox/server/internal/remoteprotocol/alist"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRemoteProviderRPCPlaybackAndDisable(t *testing.T) {
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=0-3" {
			t.Error("missing byte range")
		}
		w.Header().Set("Content-Length", "4")
		w.Header().Set("Content-Range", "bytes 0-3/100")
		w.WriteHeader(206)
		io.WriteString(w, "film")
	}))
	defer media.Close()
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	check := func(ctx context.Context) error {
		md, _ := metadata.FromIncomingContext(ctx)
		if strings.Join(md.Get("x-provider-secret"), "") != "provider-shared-secret" {
			return status.Error(codes.Unauthenticated, "bad provider secret")
		}
		return nil
	}
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if e := check(ctx); e != nil {
			return nil, e
		}
		return next(ctx, req)
	}), grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		if e := check(stream.Context()); e != nil {
			return e
		}
		method, _ := grpc.MethodFromServerStream(stream)
		switch method {
		case "/synctv.media.alist.Alist/Login":
			var in apb.LoginReq
			if e := stream.RecvMsg(&in); e != nil {
				return e
			}
			if in.Username != "viewer" || in.GetPassword() != "account-password" {
				return status.Error(codes.Unauthenticated, "bad account")
			}
			return stream.SendMsg(&apb.LoginResp{Token: "account-token"})
		case "/synctv.media.alist.Alist/FsList":
			var in apb.FsListReq
			if e := stream.RecvMsg(&in); e != nil {
				return e
			}
			if in.Token != "account-token" {
				return status.Error(codes.Unauthenticated, "bad account")
			}
			return stream.SendMsg(&apb.FsListResp{Content: []*apb.FsListResp_FsListContent{{Name: "film.mp4", Size: 100}}})
		case "/synctv.media.alist.Alist/FsGet":
			var in apb.FsGetReq
			if e := stream.RecvMsg(&in); e != nil {
				return e
			}
			return stream.SendMsg(&apb.FsGetResp{Name: "film.mp4", RawUrl: media.URL + "/film.mp4?token=remote-token"})
		}
		return status.Error(codes.Unimplemented, "unknown RPC")
	}))
	healthpb.RegisterHealthServer(grpcServer, health.NewServer())
	go grpcServer.Serve(listener)
	defer grpcServer.Stop()
	f := newManagementFixture(t, NewMemoryRepository())
	defer f.server.sources.remote.Close()
	config := RemoteProviderConfig{ID: "fixture", Name: "Fixture", Endpoint: listener.Addr().String(), Enabled: true, Timeout: 5, Types: []string{"alist"}, Secret: "provider-shared-secret"}
	raw := f.call(t, "PUT", "/api/v1/admin/remote-providers", map[string]any{"version": 0, "providers": []RemoteProviderConfig{config}}, f.root.AccessToken, 200)
	if strings.Contains(string(raw), config.Secret) {
		t.Fatal("shared secret exposed")
	}
	raw = f.call(t, "POST", "/api/v1/admin/remote-providers/fixture/probe", nil, f.root.AccessToken, 200)
	if !strings.Contains(string(raw), `"connected":true`) {
		t.Fatalf("health %s", raw)
	}
	source, e := f.server.sources.SaveRemote(context.Background(), f.member.User, RemoteSourceInput{Instance: "fixture", Kind: "alist", Name: "Remote Library", BaseURL: media.URL, Username: "viewer", Password: "account-password"})
	if e != nil {
		t.Fatal(e)
	}
	files, e := f.server.sources.Browse(context.Background(), f.member.User.ID, source.ID, "/")
	if e != nil || len(files) != 1 {
		t.Fatalf("browse %+v %v", files, e)
	}
	ticket, e := f.server.sources.PrepareMediaTicket(context.Background(), f.member.User.ID, source.ID, files[0].Path)
	if e != nil {
		t.Fatal(e)
	}
	response, e := f.server.sources.Open(context.Background(), ticket, "GET", "bytes=0-3", "")
	if e != nil {
		t.Fatal(e)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "film" {
		t.Fatal(string(body))
	}
	config.Enabled = false
	config.Secret = ""
	f.call(t, "PUT", "/api/v1/admin/remote-providers", map[string]any{"version": 1, "providers": []RemoteProviderConfig{config}}, f.root.AccessToken, 200)
	if _, e = f.server.sources.Open(context.Background(), ticket, "GET", "bytes=0-3", ""); e == nil {
		t.Fatal("disabled provider still served media")
	}
}
func TestSliceCacheIsolationExpiryPurgeAndRevocation(t *testing.T) {
	f := newManagementFixture(t, NewMemoryRepository())
	cache, e := NewSliceCache(t.TempDir(), "fixture")
	if e != nil {
		t.Fatal(e)
	}
	f.server.sources.SetSliceCache(cache)
	f.call(t, "PUT", "/api/v1/admin/slice-cache", map[string]any{"version": 0, "enabled": true, "max_bytes": 8 << 20, "ttl_seconds": 60}, f.root.AccessToken, 200)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		user, _, _ := r.BasicAuth()
		w.Header().Set("Content-Length", "4")
		w.Header().Set("Content-Range", "bytes 0-3/10")
		w.WriteHeader(206)
		if user == "one" {
			io.WriteString(w, "ONE!")
		} else {
			io.WriteString(w, "TWO!")
		}
	}))
	defer upstream.Close()
	source, e := f.server.sources.CreateWebDAV(context.Background(), f.member.User, "One", upstream.URL, "one", "secret")
	if e != nil {
		t.Fatal(e)
	}
	second, e := f.server.sources.CreateWebDAV(context.Background(), f.operator.User, "Two", upstream.URL, "two", "secret")
	if e != nil {
		t.Fatal(e)
	}
	read := func(s MediaSource) string {
		t.Helper()
		ticket, e := f.server.sources.PrepareMediaTicket(context.Background(), s.UserID, s.ID, "/film.mp4")
		if e != nil {
			t.Fatal(e)
		}
		res, e := f.server.sources.Open(context.Background(), ticket, "GET", "bytes=0-3", "")
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return string(raw)
	}
	if read(source) != "ONE!" || read(source) != "ONE!" || read(second) != "TWO!" || calls != 2 {
		t.Fatalf("cache isolation failed calls=%d", calls)
	}
	f.call(t, "PUT", "/api/v1/admin/slice-cache", map[string]any{"version": 1, "enabled": true, "max_bytes": 8 << 20, "ttl_seconds": 60, "purge": true}, f.root.AccessToken, 200)
	read(source)
	if calls != 3 {
		t.Fatal("purge didn't evict")
	}
	ticket, e := f.server.sources.PrepareMediaTicket(context.Background(), source.UserID, source.ID, "/film.mp4")
	if e != nil {
		t.Fatal(e)
	}
	f.call(t, "PUT", "/api/v1/admin/media-sources/"+source.ID, map[string]any{"version": 1, "action": "disable", "reason": "revoked"}, f.root.AccessToken, 200)
	if _, e = f.server.sources.Open(context.Background(), ticket, "GET", "bytes=0-3", ""); e == nil {
		t.Fatal("cache bypassed source revocation")
	}
	if cache.Stats().Hits != 1 {
		t.Fatalf("cache stats %+v", cache.Stats())
	}
}
func TestRTMPKeyAuthorizationRotationAndProxy(t *testing.T) {
	f := newManagementFixture(t, NewMemoryRepository())
	var streamPath string
	mtx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pw, ok := r.BasicAuth()
		if !ok || user != "sameframe" || pw != strings.Repeat("m", 32) {
			w.WriteHeader(403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v3/") {
			json.NewEncoder(w).Encode(map[string]any{"pageCount": 1, "items": []any{}})
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/"+streamPath) {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		io.WriteString(w, "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\nsegment.ts\n")
	}))
	defer mtx.Close()
	service, e := NewRTMPService(f.repo, mtx.URL, mtx.URL, "rtmp://live.example.test/sameframe", strings.Repeat("m", 32))
	if e != nil {
		t.Fatal(e)
	}
	f.server.options.RTMP = service
	f.server.sources.rtmp = service
	room := f.room(t)
	f.call(t, "POST", "/api/v1/rooms/"+room.Room.Code+"/rtmp", nil, f.operator.AccessToken, 403)
	var key struct {
		Key      string `json:"stream_key"`
		SourceID string `json:"source_id"`
	}
	json.Unmarshal(f.call(t, "POST", "/api/v1/rooms/"+room.Room.Code+"/rtmp", nil, f.member.AccessToken, 200), &key)
	u, _ := url.Parse("rtmp://host/sameframe/" + key.Key)
	streamPath = strings.TrimPrefix(u.Path, "/")
	authBody := map[string]string{"action": "publish", "protocol": "rtmp", "path": streamPath, "query": u.RawQuery}
	rawAuth, _ := json.Marshal(authBody)
	authReq, _ := http.NewRequest("POST", f.http.URL+"/internal/rtmp/auth?hook="+strings.Repeat("m", 32), strings.NewReader(string(rawAuth)))
	authRes, e := http.DefaultClient.Do(authReq)
	if e != nil {
		t.Fatal(e)
	}
	authRes.Body.Close()
	if authRes.StatusCode != 204 {
		t.Fatal("valid publish key rejected")
	}
	authBody["query"] = "token=wrong"
	req, _ := http.NewRequest("POST", f.http.URL+"/internal/rtmp/auth?hook="+strings.Repeat("m", 32), strings.NewReader(`{"action":"publish","protocol":"rtmp","path":"`+streamPath+`","query":"token=wrong"}`))
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("invalid publish key accepted")
	}
	ticket, e := f.server.sources.PrepareMediaTicket(context.Background(), f.member.User.ID, key.SourceID, "/live")
	if e != nil || !ticket.IsLive {
		t.Fatalf("RTMP ticket %+v %v", ticket, e)
	}
	res, e = f.server.sources.Open(context.Background(), ticket, "GET", "", "")
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("HLS unavailable")
	}
	f.call(t, "DELETE", "/api/v1/rooms/"+room.Room.Code+"/rtmp", nil, f.member.AccessToken, 200)
	if _, e = f.server.sources.Open(context.Background(), ticket, "GET", "", ""); e == nil {
		t.Fatal("revoked RTMP stream still playable")
	}
}
func TestSliceCacheTTL(t *testing.T) {
	cache, e := NewSliceCache(t.TempDir(), "test")
	if e != nil {
		t.Fatal(e)
	}
	cfg := CacheConfig{Enabled: true, MaxBytes: 8 << 20, TTL: 1}
	fetch := func() (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: 1, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("x"))}, nil
	}
	res, _ := cache.Open(cfg, "key", fetch)
	res.Body.Close()
	cache.mu.Lock()
	for id, e := range cache.entries {
		e.Expires = time.Now().Add(-time.Second).UnixMilli()
		cache.entries[id] = e
	}
	cache.mu.Unlock()
	res, _ = cache.Open(cfg, "key", fetch)
	res.Body.Close()
	if cache.Stats().Misses != 2 {
		t.Fatal("expired cache hit")
	}
}
