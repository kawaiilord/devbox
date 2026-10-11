package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type RTMPPolicy struct {
	Enabled bool `json:"enabled"`
	MaxTTL  int  `json:"max_ttl_seconds"`
}
type RTMPService struct {
	repo                      Repository
	api, hls, publish, secret string
	client                    *http.Client
}

func NewRTMPService(repo Repository, api, hls, publish, secret string) (*RTMPService, error) {
	if api == "" && hls == "" && publish == "" {
		return nil, nil
	}
	if len(secret) < 32 {
		return nil, errors.New("RTMP internal secret requires at least 32 characters")
	}
	for _, raw := range []string{api, hls} {
		u, e := url.Parse(raw)
		if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, errors.New("invalid MediaMTX service URL")
		}
	}
	u, e := url.Parse(publish)
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || (u.Scheme != "rtmp" && u.Scheme != "rtmps") {
		return nil, errors.New("invalid RTMP publish URL")
	}
	return &RTMPService{repo: repo, api: strings.TrimRight(api, "/"), hls: strings.TrimRight(hls, "/"), publish: strings.TrimRight(publish, "/"), secret: secret, client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (m *RTMPService) enabled(ctx context.Context) error {
	if m == nil {
		return errors.New("RTMP 服务尚未配置")
	}
	p := RTMPPolicy{Enabled: true, MaxTTL: 7200}
	if _, e := readServiceConfig(ctx, m.repo, "rtmp", &p); e != nil {
		return e
	}
	if !p.Enabled {
		return errors.New("RTMP 服务已停用")
	}
	return nil
}

var streamPathPattern = regexp.MustCompile(`^sameframe/([A-Z0-9]{6})/[A-Za-z0-9_-]{16,40}$`)

func (m *RTMPService) validStream(ctx context.Context, p string) (RoomStream, error) {
	if e := m.enabled(ctx); e != nil {
		return RoomStream{}, e
	}
	match := streamPathPattern.FindStringSubmatch(p)
	if match == nil {
		return RoomStream{}, ErrNotFound
	}
	stream, e := m.repo.ReadRoomStream(ctx, match[1])
	if e != nil || stream.Path != p || stream.Revoked || stream.ExpiresAt <= time.Now().UnixMilli() {
		return stream, ErrForbidden
	}
	room, e := m.repo.AdminReadRoom(ctx, stream.RoomCode)
	if e != nil || room.Room.Closed || room.Room.ExpiresAt <= time.Now().UnixMilli() || room.Room.OwnerID != stream.OwnerID {
		return stream, ErrForbidden
	}
	owner, e := m.repo.UserByID(ctx, stream.OwnerID)
	if e != nil || !accountActive(owner.User) {
		return stream, ErrForbidden
	}
	source, e := m.repo.GetMediaSource(ctx, stream.OwnerID, stream.SourceID)
	if e != nil || source.Disabled {
		return stream, ErrForbidden
	}
	return stream, nil
}
func (s *Server) rtmpAuth(w http.ResponseWriter, r *http.Request) {
	m := s.options.RTMP
	if m == nil || subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("hook")), []byte(m.secret)) != 1 {
		writeError(w, 403, ErrForbidden)
		return
	}
	var in struct {
		User      string `json:"user"`
		Password  string `json:"password"`
		Token     string `json:"token"`
		Action    string `json:"action"`
		Path      string `json:"path"`
		Query     string `json:"query"`
		Protocol  string `json:"protocol"`
		IP        string `json:"ip"`
		ID        string `json:"id"`
		UserAgent string `json:"userAgent"`
	}
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	if (in.Action == "api" || in.Action == "read") && in.User == "sameframe" && subtle.ConstantTimeCompare([]byte(in.Password), []byte(m.secret)) == 1 {
		if in.Action == "read" {
			if _, e := m.validStream(r.Context(), in.Path); e != nil {
				writeError(w, 403, e)
				return
			}
		}
		w.WriteHeader(204)
		return
	}
	if in.Action != "publish" || in.Protocol != "rtmp" {
		writeError(w, 403, ErrForbidden)
		return
	}
	stream, e := m.validStream(r.Context(), in.Path)
	query, qe := url.ParseQuery(in.Query)
	if e != nil || qe != nil || subtle.ConstantTimeCompare([]byte(stream.KeyHash), []byte(hashRefreshToken(query.Get("token")))) != 1 {
		writeError(w, 403, ErrForbidden)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) roomRTMP(w http.ResponseWriter, r *http.Request) {
	user, e := s.userFromRequest(r)
	if e != nil {
		writeError(w, 401, e)
		return
	}
	if !s.enforceRateLimit(w, r, "rtmp-key", user.ID, 10, time.Minute) {
		return
	}
	if e = s.options.RTMP.enabled(r.Context()); e != nil {
		writeError(w, 503, e)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	var result map[string]any
	e = s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		record, e := s.repo.AdminReadRoom(ctx, code)
		if e != nil {
			return e
		}
		room := record.Room
		if room.OwnerID != user.ID && !permissionGranted(user, "room.manage") {
			return ErrForbidden
		}
		stream, e := s.repo.ReadRoomStream(ctx, code)
		if e != nil && !errors.Is(e, ErrNotFound) {
			return e
		}
		if r.Method == "GET" {
			result = map[string]any{"active": stream.Path != "" && !stream.Revoked && stream.ExpiresAt > time.Now().UnixMilli(), "expires_at": stream.ExpiresAt, "version": stream.Version, "source_id": stream.SourceID}
			return nil
		}
		if r.Method == "DELETE" {
			if stream.Path == "" {
				return ErrNotFound
			}
			stream.Revoked = true
			stream.KeyHash = ""
			if _, e = s.repo.WriteRoomStream(ctx, stream, user.ID); e != nil {
				return e
			}
			_ = s.options.RTMP.kickPath(ctx, stream.Path)
			result = map[string]any{"revoked": true}
			return nil
		}
		if room.Closed || room.ExpiresAt <= time.Now().UnixMilli() || s.sources == nil {
			return ErrForbidden
		}
		p := RTMPPolicy{Enabled: true, MaxTTL: 7200}
		if _, e = readServiceConfig(ctx, s.repo, "rtmp", &p); e != nil {
			return e
		}
		oldPath := stream.Path
		oldOwner := stream.OwnerID
		key := mustRandomString(32)
		stream.RoomCode = code
		stream.OwnerID = room.OwnerID
		stream.Path = "sameframe/" + code + "/" + mustRandomString(16)
		stream.KeyHash = hashRefreshToken(key)
		stream.ExpiresAt = min(room.ExpiresAt, time.Now().Add(time.Duration(p.MaxTTL)*time.Second).UnixMilli())
		stream.Revoked = false
		if stream.SourceID == "" || record.Room.OwnerID != oldOwner {
			stream.SourceID = mustRandomString(12)
		}
		source, e := s.repo.GetMediaSource(ctx, room.OwnerID, stream.SourceID)
		if e != nil {
			secret, _ := json.Marshal(map[string]string{"room_code": code})
			sealed, err := s.sources.vault.Encrypt(secret, sourceAAD(room.OwnerID, stream.SourceID))
			if err != nil {
				return err
			}
			source = MediaSource{ID: stream.SourceID, UserID: room.OwnerID, Type: "rtmp", Name: "房间直播 · " + room.Name, BaseURL: "rtmp://room/" + code, CredentialsCiphertext: sealed, CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli()}
			if e = s.repo.CreateMediaSource(ctx, source); e != nil {
				return e
			}
		}
		if stream, e = s.repo.WriteRoomStream(ctx, stream, user.ID); e != nil {
			return e
		}
		if oldPath != "" {
			_ = s.options.RTMP.kickPath(ctx, oldPath)
		}
		f := record.Features
		entry := PlaylistEntry{ID: mustRandomString(10), Title: "直播 · " + room.Name, Sources: []PlaylistSource{{ID: mustRandomString(10), Label: "RTMP 直播", IsLive: true, OwnerID: room.OwnerID, MediaSourceID: source.ID, MediaPath: "/live"}}}
		entry.SelectedSourceID = entry.Sources[0].ID
		kept := []PlaylistEntry{}
		for _, item := range f.Playlist {
			has := false
			for _, src := range item.Sources {
				if src.MediaSourceID == source.ID {
					has = true
				}
			}
			if !has {
				kept = append(kept, item)
			}
		}
		f.Playlist = kept
		f.Playlist = append(f.Playlist, entry)
		f.ActiveItemID = entry.ID
		playback := room.Playback
		playback.SourceVersion++
		playback.Position = 0
		playback.PositionTS = time.Now().UnixMilli()
		playback.Playing = true
		if e = s.saveSelectedMedia(ctx, room, f, RoomMediaSelection{MediaSourceID: source.ID, MediaPath: "/live", Playback: playback}); e != nil {
			return e
		}
		result = map[string]any{"publish_url": s.options.RTMP.publish, "stream_key": strings.TrimPrefix(stream.Path, "sameframe/") + "?token=" + url.QueryEscape(key), "expires_at": stream.ExpiresAt, "source_id": source.ID, "version": stream.Version}
		return nil
	})
	if e != nil {
		writeRoomFeatureError(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, apiResponse{Code: 0, Data: result})
}

type RTMPConnection struct {
	ID            string `json:"id"`
	Created       string `json:"created"`
	RemoteAddr    string `json:"remoteAddr"`
	State         string `json:"state"`
	Path          string `json:"path"`
	InboundBytes  int64  `json:"inboundBytes"`
	OutboundBytes int64  `json:"outboundBytes"`
}

func (m *RTMPService) request(ctx context.Context, method, p string) (*http.Response, error) {
	req, e := http.NewRequestWithContext(ctx, method, m.api+p, nil)
	if e != nil {
		return nil, e
	}
	req.SetBasicAuth("sameframe", m.secret)
	return m.client.Do(req)
}
func (m *RTMPService) connections(ctx context.Context) ([]RTMPConnection, error) {
	result := []RTMPConnection{}
	for _, protocol := range []string{"rtmp", "rtmps"} {
		items, e := m.protocolConnections(ctx, protocol)
		if e != nil {
			return nil, e
		}
		result = append(result, items...)
	}
	return result, nil
}
func (m *RTMPService) protocolConnections(ctx context.Context, protocol string) ([]RTMPConnection, error) {
	items := []RTMPConnection{}
	for page := 0; page < 100; page++ {
		res, e := m.request(ctx, "GET", "/v3/"+protocol+"/conns/list?itemsPerPage=100&page="+strconv.Itoa(page))
		if e != nil {
			return nil, e
		}
		var data struct {
			Pages int              `json:"pageCount"`
			Items []RTMPConnection `json:"items"`
		}
		e = json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&data)
		res.Body.Close()
		if res.StatusCode != 200 || e != nil {
			return nil, errors.New("MediaMTX 控制 API 不可用")
		}
		for _, c := range data.Items {
			if streamPathPattern.MatchString(c.Path) {
				items = append(items, c)
			}
		}
		if page+1 >= data.Pages {
			return items, nil
		}
	}
	return nil, errors.New("RTMP 连接数量超出管理上限")
}
func (m *RTMPService) kick(ctx context.Context, id string) error {
	if !regexp.MustCompile(`^[a-fA-F0-9-]{36}$`).MatchString(id) {
		return ErrForbidden
	}
	for _, protocol := range []string{"rtmp", "rtmps"} {
		res, e := m.request(ctx, "POST", "/v3/"+protocol+"/conns/kick/"+id)
		if e != nil {
			return e
		}
		res.Body.Close()
		if res.StatusCode == 200 {
			return nil
		}
		if res.StatusCode != 404 {
			return errors.New("无法断开推流连接")
		}
	}
	return nil
}
func (m *RTMPService) kickPath(ctx context.Context, p string) error {
	items, e := m.connections(ctx)
	if e != nil {
		return e
	}
	for _, c := range items {
		if c.Path == p {
			if e = m.kick(ctx, c.ID); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Server) rtmpWatchdog(ctx context.Context) {
	if s.options.RTMP == nil {
		return
	}
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			limit, cancel := context.WithTimeout(ctx, 10*time.Second)
			items, e := s.options.RTMP.connections(limit)
			if e == nil {
				for _, c := range items {
					if _, e = s.options.RTMP.validStream(limit, c.Path); e != nil {
						_ = s.options.RTMP.kick(limit, c.ID)
					}
				}
			}
			cancel()
		}
	}
}
func (s *Server) adminRTMP(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "config.write")
	if !ok {
		return
	}
	p := RTMPPolicy{Enabled: true, MaxTTL: 7200}
	version, e := readServiceConfig(r.Context(), s.repo, "rtmp", &p)
	if e != nil {
		writeAdminError(w, e)
		return
	}
	if r.Method == "PUT" {
		var in struct {
			Version int64      `json:"version"`
			Policy  RTMPPolicy `json:"policy"`
		}
		if e = decodeJSON(r, &in); e != nil {
			writeError(w, 400, e)
			return
		}
		if in.Policy.MaxTTL < 60 || in.Policy.MaxTTL > 86400 {
			writeError(w, 400, errors.New("推流密钥有效期需为 60–86400 秒"))
			return
		}
		raw, _ := json.Marshal(in.Policy)
		d, e := s.repo.PutAdminDocument(r.Context(), AdminDocument{Key: "rtmp", Version: in.Version, Data: raw}, actor.ID)
		if e != nil {
			writeAdminError(w, e)
			return
		}
		p = in.Policy
		version = d.Version
	}
	items := []RTMPConnection{}
	probeError := ""
	publish := ""
	if m := s.options.RTMP; m != nil {
		publish = m.publish
		items, e = m.connections(r.Context())
		if e != nil {
			items = []RTMPConnection{}
			probeError = "MediaMTX 控制 API 尚未连接"
		}
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"version": version, "policy": p, "configured": s.options.RTMP != nil, "publish_url": publish, "connections": items, "error": probeError}})
}
func (s *Server) adminRTMPKick(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "room.manage")
	if !ok {
		return
	}
	m := s.options.RTMP
	if m == nil {
		writeError(w, 503, ErrNotFound)
		return
	}
	items, e := m.connections(r.Context())
	if e != nil {
		writeError(w, 503, errors.New("MediaMTX 连接不可用"))
		return
	}
	for _, c := range items {
		if c.ID != r.PathValue("id") {
			continue
		}
		stream, e := s.repo.ReadRoomStream(r.Context(), streamPathPattern.FindStringSubmatch(c.Path)[1])
		if e == nil && stream.Path == c.Path {
			stream.Revoked = true
			stream.KeyHash = ""
			_, e = s.repo.WriteRoomStream(r.Context(), stream, actor.ID)
		}
		if e == nil {
			e = m.kick(r.Context(), c.ID)
		}
		if e != nil {
			writeAdminError(w, e)
			return
		}
		writeJSON(w, 200, apiResponse{Code: 0, Msg: "推流已断开，原密钥已撤销"})
		return
	}
	writeError(w, 404, ErrNotFound)
}
func (m *MediaSourceManager) prepareRTMP(ctx context.Context, source MediaSource, secret []byte) (MediaTicket, error) {
	if e := m.rtmp.enabled(ctx); e != nil {
		return MediaTicket{}, e
	}
	var ref struct {
		RoomCode string `json:"room_code"`
	}
	if json.Unmarshal(secret, &ref) != nil {
		return MediaTicket{}, ErrForbidden
	}
	stream, e := m.repository.ReadRoomStream(ctx, ref.RoomCode)
	if e != nil {
		return MediaTicket{}, e
	}
	if _, e = m.rtmp.validStream(ctx, stream.Path); e != nil || stream.OwnerID != source.UserID || stream.SourceID != source.ID {
		return MediaTicket{}, ErrForbidden
	}
	return MediaTicket{UserID: source.UserID, SourceID: source.ID, Path: "/live", StreamURL: m.rtmp.hls + "/" + stream.Path + "/index.m3u8", Kind: "platform-hls", IsLive: true}, nil
}
func (m *MediaSourceManager) openRTMP(ctx context.Context, source MediaSource, secret []byte, ticket MediaTicket, method, rng string) (*http.Response, error) {
	base, e := m.prepareRTMP(ctx, source, secret)
	if e != nil {
		return nil, e
	}
	root := strings.TrimSuffix(base.StreamURL, "index.m3u8")
	u, e := url.Parse(ticket.StreamURL)
	if e != nil || !strings.HasPrefix(u.String(), root) || strings.Contains(u.Path, "..") || u.User != nil {
		return nil, ErrForbidden
	}
	req, e := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if e != nil {
		return nil, e
	}
	req.SetBasicAuth("sameframe", m.rtmp.secret)
	if rng != "" {
		req.Header.Set("Range", rng)
	}
	client := *m.rtmp.client
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 || next.URL.User != nil || strings.Contains(next.URL.Path, "..") || !strings.HasPrefix(next.URL.String(), root) {
			return ErrForbidden
		}
		next.SetBasicAuth("sameframe", m.rtmp.secret)
		return nil
	}
	return client.Do(req)
}
