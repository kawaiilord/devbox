package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	apb "github.com/kawaiilord/devbox/server/internal/remoteprotocol/alist"
	bpb "github.com/kawaiilord/devbox/server/internal/remoteprotocol/bilibili"
	epb "github.com/kawaiilord/devbox/server/internal/remoteprotocol/emby"
	"google.golang.org/protobuf/proto"
)

type remoteSourceSecret struct {
	Instance string            `json:"instance"`
	Token    string            `json:"token"`
	UserID   string            `json:"user_id"`
	Cookies  map[string]string `json:"cookies"`
}
type RemoteSourceInput struct {
	Instance string `json:"instance"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	Username string `json:"username"`
	Password string `json:"password"`
	Cookie   string `json:"cookie"`
	OTP      string `json:"otp"`
}

func remoteKind(source MediaSource) string { return strings.TrimPrefix(source.Type, "remote-") }
func (m *MediaSourceManager) remoteCall(ctx context.Context, c remoteSourceSecret, kind, method string, in, out proto.Message) error {
	services := map[string]string{"alist": "synctv.media.alist.Alist", "emby": "synctv.media.emby.Emby", "bilibili": "synctv.media.bilibili.Bilibili"}
	service, ok := services[kind]
	if !ok {
		return ErrForbidden
	}
	return m.remote.invoke(ctx, c.Instance, kind, "/"+service+"/"+method, in, out)
}
func (m *MediaSourceManager) SaveRemote(ctx context.Context, user User, in RemoteSourceInput) (MediaSource, error) {
	if len([]rune(in.Name)) < 1 || len([]rune(in.Name)) > 64 || len(in.Password) > 2048 || len(in.Cookie) > 16000 {
		return MediaSource{}, errors.New("媒体源名称或凭据长度无效")
	}
	if _, e := m.remote.config(ctx, in.Instance, in.Kind); e != nil {
		return MediaSource{}, e
	}
	c := remoteSourceSecret{Instance: in.Instance}
	base := "https://www.bilibili.com"
	if in.Kind != "bilibili" {
		u, e := m.validateBaseURL(ctx, in.BaseURL)
		if e != nil {
			return MediaSource{}, e
		}
		base = strings.TrimRight(u.String(), "/")
	}
	switch in.Kind {
	case "alist":
		var result apb.LoginResp
		e := m.remoteCall(ctx, c, in.Kind, "Login", &apb.LoginReq{Host: base, Username: in.Username, Credential: &apb.LoginReq_Password{Password: in.Password}, OtpCode: in.OTP}, &result)
		if e != nil {
			return MediaSource{}, e
		}
		c.Token = result.Token
	case "emby":
		var result epb.LoginResp
		e := m.remoteCall(ctx, c, in.Kind, "Login", &epb.LoginReq{Host: base, Username: in.Username, Credential: &epb.LoginReq_Password{Password: in.Password}}, &result)
		if e != nil {
			return MediaSource{}, e
		}
		c.Token = result.Token
		c.UserID = result.UserId
	case "bilibili":
		cookies, e := http.ParseCookie(in.Cookie)
		if e != nil {
			return MediaSource{}, errors.New("Cookie 格式无效")
		}
		c.Cookies = map[string]string{}
		for _, cookie := range cookies {
			c.Cookies[cookie.Name] = cookie.Value
		}
	default:
		return MediaSource{}, ErrForbidden
	}
	id := mustRandomString(12)
	raw, _ := json.Marshal(c)
	sealed, e := m.vault.Encrypt(raw, sourceAAD(user.ID, id))
	if e != nil {
		return MediaSource{}, e
	}
	now := time.Now().UnixMilli()
	source := MediaSource{ID: id, UserID: user.ID, Type: "remote-" + in.Kind, Name: in.Name, BaseURL: base, CredentialsCiphertext: sealed, CreatedAt: now, UpdatedAt: now}
	return source, m.repository.CreateMediaSource(ctx, source)
}
func (s *Server) saveRemoteSource(w http.ResponseWriter, r *http.Request) {
	user, ok := s.providerOwner(w, r)
	if !ok {
		return
	}
	var in RemoteSourceInput
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	source, e := s.sources.SaveRemote(r.Context(), user, in)
	if e != nil {
		writeRoomFeatureError(w, e)
		return
	}
	writeJSON(w, 201, apiResponse{Code: 0, Data: source})
}
func decodeRemoteSecret(secret []byte) (remoteSourceSecret, error) {
	var c remoteSourceSecret
	e := json.Unmarshal(secret, &c)
	if e == nil && c.Instance == "" {
		e = ErrForbidden
	}
	return c, e
}
func (m *MediaSourceManager) browseRemote(ctx context.Context, source MediaSource, secret []byte, p string) ([]MediaFile, error) {
	c, e := decodeRemoteSecret(secret)
	if e != nil {
		return nil, e
	}
	kind := remoteKind(source)
	files := []MediaFile{}
	switch kind {
	case "alist":
		var out apb.FsListResp
		e = m.remoteCall(ctx, c, kind, "FsList", &apb.FsListReq{Host: source.BaseURL, Token: c.Token, Path: cleanMediaPath(p), Page: 1, PerPage: 500}, &out)
		if e != nil {
			return nil, e
		}
		for _, f := range out.Content {
			if f.Name == "." || f.Name == ".." || strings.Contains(f.Name, "/") {
				continue
			}
			files = append(files, MediaFile{Name: f.Name, Path: path.Join(cleanMediaPath(p), f.Name), IsDirectory: f.IsDir, Size: int64(f.Size)})
		}
	case "emby":
		var out epb.GetItemsResp
		e = m.remoteCall(ctx, c, kind, "GetItems", &epb.GetItemsReq{Host: source.BaseURL, Token: c.Token, UserId: c.UserID, ParentId: strings.Trim(p, "/")}, &out)
		if e != nil {
			return nil, e
		}
		for _, f := range out.Items {
			files = append(files, MediaFile{Name: f.Name, Path: "/" + f.Id, IsDirectory: f.IsFolder})
		}
	case "bilibili":
		return files, nil
	default:
		return nil, ErrForbidden
	}
	return files, nil
}

type remoteBiliMedia struct {
	BVID string `json:"bvid"`
	AID  uint64 `json:"aid"`
	CID  uint64 `json:"cid"`
}

func biliMediaPath(m remoteBiliMedia) string {
	raw, _ := json.Marshal(m)
	return "/remote-video/" + base64.RawURLEncoding.EncodeToString(raw)
}
func parseRemoteBiliPath(p string) (remoteBiliMedia, error) {
	var m remoteBiliMedia
	if !strings.HasPrefix(p, "/remote-video/") || len(p) > 2000 {
		return m, ErrNotFound
	}
	raw, e := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(p, "/remote-video/"))
	if e != nil {
		return m, e
	}
	e = json.Unmarshal(raw, &m)
	if e == nil && (m.CID == 0 || m.BVID == "" && m.AID == 0) {
		e = ErrNotFound
	}
	return m, e
}
func (m *MediaSourceManager) resolveRemoteBili(ctx context.Context, source MediaSource, secret []byte, raw string) ([]MediaFile, error) {
	c, e := decodeRemoteSecret(secret)
	if e != nil {
		return nil, e
	}
	target, e := canonicalPlatformURL("bilibili", raw)
	if e != nil {
		return nil, e
	}
	u, _ := url.Parse(target)
	id := regexp.MustCompile(`(?:BV[A-Za-z0-9]{10}|av[0-9]+)`).FindString(u.Path)
	if id == "" {
		return nil, errors.New("远程 B站入口当前支持 BV 或 av 视频链接")
	}
	in := &bpb.ParseVideoPageReq{Cookies: c.Cookies}
	if strings.HasPrefix(id, "av") {
		in.Aid, _ = strconv.ParseUint(id[2:], 10, 64)
	} else {
		in.Bvid = id
	}
	var out bpb.VideoPageInfo
	if e = m.remoteCall(ctx, c, "bilibili", "ParseVideoPage", in, &out); e != nil {
		return nil, e
	}
	files := []MediaFile{}
	for i, f := range out.VideoInfos {
		name := f.Name
		if name == "" {
			name = fmt.Sprintf("%s · 第 %d 集", out.Title, i+1)
		}
		files = append(files, MediaFile{Name: name, Path: biliMediaPath(remoteBiliMedia{BVID: f.Bvid, AID: f.Aid, CID: f.Cid}), ContentType: "video/mp4"})
	}
	return files, nil
}
func (m *MediaSourceManager) remoteBiliDash(ctx context.Context, source MediaSource, secret []byte, p string) (*bpb.DashInfo, error) {
	c, e := decodeRemoteSecret(secret)
	if e != nil {
		return nil, e
	}
	media, e := parseRemoteBiliPath(p)
	if e != nil {
		return nil, e
	}
	var out bpb.GetDashVideoURLResp
	e = m.remoteCall(ctx, c, "bilibili", "GetDashVideoURL", &bpb.GetDashVideoURLReq{Cookies: c.Cookies, Bvid: media.BVID, Aid: media.AID, Cid: media.CID}, &out)
	if e != nil {
		return nil, e
	}
	if out.Dash == nil || len(out.Dash.VideoStreams) == 0 || len(out.Dash.AudioStreams) == 0 {
		return nil, errors.New("远程 B站未返回可用音视频流")
	}
	return out.Dash, nil
}
func (m *MediaSourceManager) remoteVariants(ctx context.Context, source MediaSource, secret []byte, p string) ([]MediaVariant, error) {
	if source.Type != "remote-bilibili" {
		return []MediaVariant{{ID: "original", Label: "原画", Protocol: "file"}}, nil
	}
	dash, e := m.remoteBiliDash(ctx, source, secret, p)
	if e != nil {
		return nil, e
	}
	items := []MediaVariant{}
	seen := map[uint64]bool{}
	for _, v := range dash.VideoStreams {
		if !seen[v.Id] {
			name := v.QualityName
			if name == "" {
				name = fmt.Sprintf("%dp", v.Height)
			}
			items = append(items, MediaVariant{ID: strconv.FormatUint(v.Id, 10), Label: name, Height: int(v.Height), Protocol: "dash"})
			seen[v.Id] = true
		}
	}
	return items, nil
}
func (m *MediaSourceManager) prepareRemote(ctx context.Context, source MediaSource, secret []byte, p, variant string) (MediaTicket, error) {
	c, e := decodeRemoteSecret(secret)
	if e != nil {
		return MediaTicket{}, e
	}
	ticket := MediaTicket{UserID: source.UserID, SourceID: source.ID, Path: p, Kind: "platform-file", VariantID: variant}
	kind := remoteKind(source)
	switch kind {
	case "alist":
		var out apb.FsGetResp
		e = m.remoteCall(ctx, c, kind, "FsGet", &apb.FsGetReq{Host: source.BaseURL, Token: c.Token, Path: cleanMediaPath(p)}, &out)
		if e != nil {
			return ticket, e
		}
		if out.IsDir {
			return ticket, ErrNotFound
		}
		ticket.StreamURL = out.RawUrl
	case "emby":
		var out epb.PlaybackInfoResp
		e = m.remoteCall(ctx, c, kind, "PlaybackInfo", &epb.PlaybackInfoReq{Host: source.BaseURL, Token: c.Token, UserId: c.UserID, ItemId: strings.Trim(p, "/"), EnableDirectPlay: proto.Bool(true), EnableDirectStream: proto.Bool(true), EnableTranscoding: proto.Bool(false), AudioStreamIndex: -1, SubtitleStreamIndex: -1}, &out)
		if e != nil {
			return ticket, e
		}
		if len(out.MediaSourceInfo) == 0 {
			return ticket, ErrNotFound
		}
		media := out.MediaSourceInfo[0]
		base, _ := url.Parse(source.BaseURL + "/")
		target := media.DirectPlayUrl
		if target == "" {
			target = "Videos/" + url.PathEscape(strings.Trim(p, "/")) + "/stream?Static=true&MediaSourceId=" + url.QueryEscape(media.Id)
		}
		u, e := base.Parse(target)
		if e != nil {
			return ticket, e
		}
		if u.Host != base.Host {
			return ticket, errors.New("Emby 播放地址跨越服务器边界")
		}
		q := u.Query()
		q.Set("api_key", c.Token)
		u.RawQuery = q.Encode()
		ticket.StreamURL = u.String()
		ticket.Container = media.Container
	case "bilibili":
		dash, e := m.remoteBiliDash(ctx, source, secret, p)
		if e != nil {
			return ticket, e
		}
		video := dash.VideoStreams[0]
		if variant != "" && variant != "original" {
			found := false
			for _, v := range dash.VideoStreams {
				if strconv.FormatUint(v.Id, 10) == variant {
					video = v
					found = true
					break
				}
			}
			if !found {
				return ticket, ErrNotFound
			}
		}
		audio := dash.AudioStreams[0]
		ticket.Kind = "platform-dash"
		ticket.StreamURL = video.BaseUrl
		ticket.AudioURL = audio.BaseUrl
		ticket.Duration = dash.Duration
		ticket.VideoCodec = video.Codecs
		ticket.AudioCodec = audio.Codecs
		ticket.Width = int(video.Width)
		ticket.Height = int(video.Height)
		ticket.Bandwidth = int64(video.Bandwidth)
		ticket.StreamHeaders = map[string]string{"Referer": "https://www.bilibili.com/", "User-Agent": "Mozilla/5.0"}
	default:
		return ticket, ErrForbidden
	}
	if _, e = m.validateRemoteMediaURL(ctx, ticket.StreamURL); e != nil {
		return ticket, e
	}
	u, _ := url.Parse(ticket.StreamURL)
	if strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") {
		ticket.Kind = "platform-hls"
	}
	if strings.HasSuffix(strings.ToLower(u.Path), ".flv") {
		ticket.Container = "flv"
	}
	return ticket, nil
}
func (m *MediaSourceManager) openRemote(ctx context.Context, source MediaSource, secret []byte, ticket MediaTicket, method, rng, ifRange string) (*http.Response, error) {
	c, e := decodeRemoteSecret(secret)
	if e != nil {
		return nil, e
	}
	if _, e = m.remote.config(ctx, c.Instance, remoteKind(source)); e != nil {
		return nil, e
	}
	if method != "GET" && method != "HEAD" {
		return nil, ErrForbidden
	}
	if _, e = m.validateRemoteMediaURL(ctx, ticket.StreamURL); e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, method, ticket.StreamURL, nil)
	if e != nil {
		return nil, e
	}
	for k, v := range safePlatformHeaders(ticket.StreamHeaders) {
		req.Header.Set(k, v)
	}
	if rng != "" {
		req.Header.Set("Range", rng)
	}
	if ifRange != "" {
		req.Header.Set("If-Range", ifRange)
	}
	return m.client.Do(req)
}

func (m *MediaSourceManager) validateRemoteMediaURL(ctx context.Context, raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(raw) > 16384 {
		return nil, errors.New("媒体地址无效")
	}
	allow := m.allowPrivate || m.trustedAuthorities[sourceAuthority(u)]
	if u.Scheme != "https" && !(allow && u.Scheme == "http") {
		return nil, errors.New("媒体地址必须使用 HTTPS")
	}
	if e = validateSourceHost(ctx, u.Hostname(), allow); e != nil {
		return nil, e
	}
	return u, nil
}
