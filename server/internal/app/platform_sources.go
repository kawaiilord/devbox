package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type platformDefinition struct {
	Name, Origin, Extractor string
	Hosts                   []string
}

var platformDefinitions = map[string]platformDefinition{
	"bilibili": {"哔哩哔哩", "https://www.bilibili.com/", "(?i)bili.*", []string{"bilibili.com", "b23.tv"}},
	"youtube":  {"YouTube", "https://www.youtube.com/", "(?i)youtube.*", []string{"youtube.com", "youtu.be"}},
	"douyin":   {"抖音", "https://www.douyin.com/", "(?i)douyin", []string{"douyin.com", "iesdouyin.com"}},
	"tiktok":   {"TikTok", "https://www.tiktok.com/", "(?i)tiktok.*", []string{"tiktok.com"}},
	"twitch":   {"Twitch", "https://www.twitch.tv/", "(?i)twitch.*", []string{"twitch.tv"}},
	"huya":     {"虎牙", "https://www.huya.com/", "(?i)huya.*", []string{"huya.com"}},
	"douyu":    {"斗鱼", "https://www.douyu.com/", "(?i)douyu.*", []string{"douyu.com"}},
	"acfun":    {"AcFun", "https://www.acfun.cn/", "(?i)acfun.*", []string{"acfun.cn"}},
	"cctv":     {"央视", "https://tv.cctv.com/", "(?i)cctv", []string{"cctv.com", "cntv.cn"}},
}

type platformCredentials struct {
	Cookie string `json:"cookie,omitempty"`
}
type platformFormat struct {
	ID         string            `json:"format_id"`
	URL        string            `json:"url"`
	Ext        string            `json:"ext"`
	Protocol   string            `json:"protocol"`
	VideoCodec string            `json:"vcodec"`
	AudioCodec string            `json:"acodec"`
	Height     int               `json:"height"`
	Width      int               `json:"width"`
	Bitrate    float64           `json:"tbr"`
	FPS        float64           `json:"fps"`
	HasDRM     bool              `json:"has_drm"`
	Headers    map[string]string `json:"http_headers"`
}
type platformInfo struct {
	Type       string            `json:"_type"`
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	URL        string            `json:"url"`
	WebpageURL string            `json:"webpage_url"`
	Duration   float64           `json:"duration"`
	IsLive     bool              `json:"is_live"`
	Formats    []platformFormat  `json:"formats"`
	Entries    []platformInfo    `json:"entries"`
	Headers    map[string]string `json:"http_headers"`
}
type platformCachedInfo struct {
	info    platformInfo
	expires time.Time
}
type platformResolver struct {
	path  string
	slots chan struct{}
	mu    sync.Mutex
	cache map[string]platformCachedInfo
	run   func(context.Context, string, string, string, bool) (platformInfo, error)
}

func newPlatformResolver() *platformResolver {
	name := os.Getenv("SAMEFRAME_YTDLP_PATH")
	if name == "" {
		name = "yt-dlp"
	}
	resolved, _ := exec.LookPath(name)
	return &platformResolver{path: resolved, slots: make(chan struct{}, 2), cache: map[string]platformCachedInfo{}}
}
func (r *platformResolver) available() bool { return r != nil && (r.path != "" || r.run != nil) }

func canonicalPlatformURL(provider, raw string) (string, error) {
	d, ok := platformDefinitions[provider]
	if !ok {
		return "", errors.New("平台不受支持")
	}
	raw = strings.TrimSpace(raw)
	if provider == "bilibili" && regexp.MustCompile(`^(BV[A-Za-z0-9]{10}|av[0-9]+)$`).MatchString(raw) {
		raw = d.Origin + "video/" + raw
	}
	if provider == "youtube" && regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`).MatchString(raw) {
		raw = d.Origin + "watch?v=" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || (u.Port() != "" && u.Port() != "443") {
		return "", errors.New("请输入所选平台的完整视频或播放列表链接")
	}
	host := strings.ToLower(u.Hostname())
	allowed := false
	for _, h := range d.Hosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			allowed = true
		}
	}
	if !allowed {
		return "", errors.New("链接域名与所选平台不一致")
	}
	u.Scheme = "https"
	u.Fragment = ""
	return u.String(), nil
}

func platformMediaPath(raw string) string {
	return "/video/" + base64.RawURLEncoding.EncodeToString([]byte(raw))
}
func platformPathURL(provider, p string) (string, error) {
	if !strings.HasPrefix(p, "/video/") {
		return "", errors.New("请先解析平台视频链接")
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(p, "/video/"))
	if err != nil {
		return "", errors.New("平台媒体标识无效")
	}
	return canonicalPlatformURL(provider, string(b))
}

func normalizePlatformCookie(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.HasPrefix(strings.ToLower(raw), "cookie:") {
		raw = strings.TrimSpace(raw[7:])
	}
	if len(raw) > 16384 || strings.ContainsAny(raw, "\r\n\x00") {
		return "", errors.New("请粘贴单行 Cookie 请求头值")
	}
	cookies, err := http.ParseCookie(raw)
	if err != nil {
		return "", errors.New("Cookie 格式无效")
	}
	parts := []string{}
	seen := map[string]bool{}
	for _, cookie := range cookies {
		if cookie.Valid() != nil || seen[cookie.Name] {
			return "", errors.New("Cookie 格式无效")
		}
		seen[cookie.Name] = true
		parts = append(parts, cookie.String())
	}
	return strings.Join(parts, "; "), nil
}

func (m *MediaSourceManager) SavePlatform(ctx context.Context, user User, sourceID, provider, name, cookie string) (MediaSource, error) {
	d, ok := platformDefinitions[provider]
	if !ok {
		return MediaSource{}, errors.New("平台不受支持")
	}
	if !m.platform.available() {
		return MediaSource{}, errors.New("服务器尚未安装平台解析组件 yt-dlp")
	}
	name = strings.TrimSpace(name)
	if len([]rune(name)) < 1 || len([]rune(name)) > 64 {
		return MediaSource{}, errors.New("名称需为 1–64 个字符")
	}
	cookie, err := normalizePlatformCookie(cookie)
	if err != nil {
		return MediaSource{}, err
	}
	var source MediaSource
	if sourceID != "" {
		source, err = m.repository.GetMediaSource(ctx, user.ID, sourceID)
		if err != nil {
			return source, err
		}
		if source.Type != provider {
			return source, ErrForbidden
		}
	} else {
		now := time.Now().UnixMilli()
		source = MediaSource{ID: mustRandomString(12), UserID: user.ID, Type: provider, Name: name, BaseURL: d.Origin, CreatedAt: now, UpdatedAt: now}
	}
	secret, _ := json.Marshal(platformCredentials{Cookie: cookie})
	encrypted, err := m.vault.Encrypt(secret, sourceAAD(user.ID, source.ID))
	if err != nil {
		return source, err
	}
	if sourceID == "" {
		source.CredentialsCiphertext = encrypted
		err = m.repository.CreateMediaSource(ctx, source)
	} else {
		var changed bool
		changed, err = m.repository.ReplaceMediaSourceCredentials(ctx, user.ID, source.ID, source.CredentialsCiphertext, encrypted)
		if err == nil && !changed {
			err = ErrRoomConflict
		}
		source.CredentialsCiphertext = encrypted
	}
	return source, err
}

type cappedOutput struct {
	bytes.Buffer
	limit int
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("provider output limit exceeded")
	}
	return b.Buffer.Write(p)
}

func (r *platformResolver) resolve(ctx context.Context, provider, target, cookie string, flat bool, offsets ...int) (platformInfo, error) {
	if !r.available() {
		return platformInfo{}, errors.New("平台解析组件未安装")
	}
	target, err := canonicalPlatformURL(provider, target)
	if err != nil {
		return platformInfo{}, err
	}
	offset := 0
	if len(offsets) > 0 {
		offset = offsets[0]
	}
	if offset < 0 || offset > 5000 {
		return platformInfo{}, errors.New("播放列表页码无效")
	}
	digest := sha256.Sum256([]byte(strconv.Itoa(offset) + provider + "\x00" + target + "\x00" + cookie + "\x00" + strconv.FormatBool(flat)))
	key := hex.EncodeToString(digest[:])
	r.mu.Lock()
	cached, ok := r.cache[key]
	r.mu.Unlock()
	if ok && cached.expires.After(time.Now()) {
		return cached.info, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if r.slots != nil {
		select {
		case r.slots <- struct{}{}:
			defer func() { <-r.slots }()
		case <-ctx.Done():
			return platformInfo{}, errors.New("平台解析繁忙，请稍后重试")
		}
	}
	var info platformInfo
	if r.run != nil {
		info, err = r.run(ctx, provider, target, cookie, flat)
	} else {
		info, err = r.execute(ctx, provider, target, cookie, flat, offset)
	}
	if err != nil {
		return platformInfo{}, err
	}
	if len(info.Formats) > 512 || len(info.Entries) > 50 {
		return platformInfo{}, errors.New("平台返回的媒体数量过多")
	}
	r.mu.Lock()
	if r.cache == nil {
		r.cache = map[string]platformCachedInfo{}
	}
	if len(r.cache) >= 32 {
		for k, v := range r.cache {
			if !v.expires.After(time.Now()) {
				delete(r.cache, k)
			}
		}
		if len(r.cache) >= 32 {
			for k := range r.cache {
				delete(r.cache, k)
				break
			}
		}
	}
	r.cache[key] = platformCachedInfo{info: info, expires: time.Now().Add(2 * time.Minute)}
	r.mu.Unlock()
	return info, nil
}

func (r *platformResolver) execute(ctx context.Context, provider, target, cookie string, flat bool, offset int) (platformInfo, error) {
	d := platformDefinitions[provider]
	args := []string{"--ignore-config", "--no-plugin-dirs", "--no-cache-dir", "--no-cookies-from-browser", "--no-remote-components", "--js-runtimes", "node", "--no-mark-watched", "--no-geo-bypass", "--skip-download", "--dump-single-json", "--no-progress", "--no-warnings", "--socket-timeout", "15", "--retries", "1", "--extractor-retries", "1", "--playlist-start", strconv.Itoa(offset + 1), "--playlist-end", strconv.Itoa(offset + 50), "--use-extractors", d.Extractor}
	if flat {
		args = append(args, "--flat-playlist")
	} else {
		args = append(args, "--no-playlist")
	}
	if cookie != "" {
		f, err := os.CreateTemp("", "sameframe-provider-*.cookies")
		if err != nil {
			return platformInfo{}, errors.New("无法建立临时登录会话")
		}
		defer os.Remove(f.Name())
		cookies, _ := http.ParseCookie(cookie)
		var b strings.Builder
		b.WriteString("# Netscape HTTP Cookie File\n")
		for _, c := range cookies {
			for _, host := range d.Hosts[:1] {
				b.WriteString("." + host + "\tTRUE\t/\tTRUE\t0\t" + c.Name + "\t" + c.Value + "\n")
			}
		}
		_, err = f.WriteString(b.String())
		f.Close()
		if err != nil {
			return platformInfo{}, errors.New("无法建立临时登录会话")
		}
		args = append(args, "--cookies", f.Name())
	}
	args = append(args, "--", target)
	command := exec.CommandContext(ctx, r.path, args...)
	command.WaitDelay = 2 * time.Second
	output := &cappedOutput{limit: 8 << 20}
	diagnostics := &cappedOutput{limit: 64 << 10}
	command.Stdout = output
	command.Stderr = diagnostics
	if err := command.Run(); err != nil {
		message := strings.ToLower(diagnostics.String())
		if strings.Contains(message, "login") || strings.Contains(message, "sign in") || strings.Contains(message, "cookie") {
			return platformInfo{}, errors.New("该视频需要登录或平台验证，请更新登录 Cookie 后重试")
		}
		return platformInfo{}, errors.New("平台解析失败，请检查链接、账号权限或服务器网络")
	}
	var info platformInfo
	if json.Unmarshal(output.Bytes(), &info) != nil {
		return info, errors.New("平台解析组件返回了无效数据")
	}
	return info, nil
}

func (m *MediaSourceManager) ResolvePlatform(ctx context.Context, userID, sourceID, raw string, offsets ...int) ([]MediaFile, error) {
	source, secret, err := m.loadSourceSecret(ctx, userID, sourceID)
	if err != nil {
		return nil, err
	}
	if _, ok := platformDefinitions[source.Type]; !ok {
		return nil, errors.New("请选择视频平台媒体源")
	}
	var c platformCredentials
	if json.Unmarshal(secret, &c) != nil {
		return nil, errors.New("平台登录数据无效")
	}
	target, err := canonicalPlatformURL(source.Type, raw)
	if err != nil {
		return nil, err
	}
	offset := 0
	if len(offsets) > 0 {
		offset = offsets[0]
	}
	info, err := m.platform.resolve(ctx, source.Type, target, c.Cookie, true, offset)
	if err != nil {
		return nil, err
	}
	entries := info.Entries
	if len(entries) == 0 && (info.Type == "playlist" || info.Type == "multi_video") {
		if offset > 0 {
			return []MediaFile{}, nil
		}
		return nil, errors.New("播放列表为空或没有可访问的影片")
	}
	if len(entries) == 0 {
		info.WebpageURL = target
		entries = []platformInfo{info}
	}
	files := []MediaFile{}
	for _, entry := range entries {
		link := entry.WebpageURL
		if link == "" {
			link = entry.URL
		}
		if link == "" {
			link = entry.ID
		}
		canonical, err := canonicalPlatformURL(source.Type, link)
		if err != nil {
			continue
		}
		name := sanitizeAuditText(entry.Title, 256)
		if name == "" {
			name = entry.ID
		}
		files = append(files, MediaFile{Name: name, Path: platformMediaPath(canonical), ContentType: "video/platform"})
	}
	if len(files) == 0 {
		return nil, errors.New("未找到可加入片单的视频")
	}
	return files, nil
}

func mediaRemoteURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || len(raw) > 16384 || (u.Scheme != "http" && u.Scheme != "https") || (u.Port() != "" && u.Port() != "443" && u.Port() != "80") || net.ParseIP(u.Hostname()) != nil {
		return nil, errors.New("平台返回了不安全的媒体地址")
	}
	return u, nil
}

func supportedPlatformFormats(info platformInfo) []platformFormat {
	formats := []platformFormat{}
	for _, f := range info.Formats {
		if f.HasDRM || f.VideoCodec == "none" || f.ID == "" || len(f.ID) > 128 {
			continue
		}
		if _, err := mediaRemoteURL(f.URL); err != nil {
			continue
		}
		if strings.Contains(f.Protocol, "m3u8") || ((f.Ext == "mp4" || f.Ext == "flv") && (f.Protocol == "https" || f.Protocol == "http" || f.Protocol == "")) {
			formats = append(formats, f)
		}
	}
	sort.SliceStable(formats, func(i, j int) bool { return formats[i].Height > formats[j].Height })
	return formats
}

func bestPlatformAudio(info platformInfo) (platformFormat, bool) {
	var result platformFormat
	found := false
	for _, f := range info.Formats {
		if f.HasDRM || f.VideoCodec != "none" || f.AudioCodec == "none" || f.AudioCodec == "" || (f.Ext != "m4a" && f.Ext != "mp4") {
			continue
		}
		if _, err := mediaRemoteURL(f.URL); err != nil {
			continue
		}
		if !found || f.Bitrate > result.Bitrate {
			result = f
			found = true
		}
	}
	return result, found
}

func (m *MediaSourceManager) platformInfo(ctx context.Context, source MediaSource, secret []byte, p string) (platformInfo, platformCredentials, error) {
	var c platformCredentials
	if json.Unmarshal(secret, &c) != nil {
		return platformInfo{}, c, errors.New("平台登录数据无效")
	}
	target, err := platformPathURL(source.Type, p)
	if err != nil {
		return platformInfo{}, c, err
	}
	info, err := m.platform.resolve(ctx, source.Type, target, c.Cookie, false)
	return info, c, err
}

func (m *MediaSourceManager) platformVariants(ctx context.Context, source MediaSource, secret []byte, p string) ([]MediaVariant, error) {
	info, _, err := m.platformInfo(ctx, source, secret, p)
	if err != nil {
		return nil, err
	}
	formats := supportedPlatformFormats(info)
	if len(formats) == 0 {
		return nil, errors.New("该视频没有可用的未加密播放格式")
	}
	_, hasAudio := bestPlatformAudio(info)
	variants := []MediaVariant{{ID: "auto", Label: "自动（推荐）"}}
	for _, f := range formats {
		if f.AudioCodec == "none" && !hasAudio {
			continue
		}
		label := f.ID
		if f.Height > 0 {
			label = strconv.Itoa(f.Height) + "p"
		}
		if f.VideoCodec != "" {
			label += " · " + strings.Split(f.VideoCodec, ".")[0]
		}
		variants = append(variants, MediaVariant{ID: f.ID, Label: label, Height: f.Height, Protocol: f.Protocol})
	}
	return variants, nil
}

func safePlatformHeaders(headers map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range headers {
		switch strings.ToLower(key) {
		case "user-agent", "referer", "accept-language":
			if len(value) < 2048 && !strings.ContainsAny(value, "\r\n\x00") {
				result[http.CanonicalHeaderKey(key)] = value
			}
		}
	}
	return result
}

func (m *MediaSourceManager) preparePlatformTicket(ctx context.Context, source MediaSource, secret []byte, p, variant string) (MediaTicket, error) {
	info, _, err := m.platformInfo(ctx, source, secret, p)
	if err != nil {
		return MediaTicket{}, err
	}
	formats := supportedPlatformFormats(info)
	if len(formats) == 0 {
		return MediaTicket{}, errors.New("没有可播放格式，请检查平台账号权限")
	}
	selected := formats[0]
	found := variant == "" || variant == "auto" || variant == "original"
	if found {
		for _, f := range formats {
			if f.Height <= 1080 && f.AudioCodec != "none" {
				selected = f
				break
			}
		}
	} else {
		for _, f := range formats {
			if f.ID == variant {
				selected = f
				found = true
				break
			}
		}
	}
	if !found {
		return MediaTicket{}, errors.New("该画质当前不可用，请刷新画质列表")
	}
	headers := safePlatformHeaders(info.Headers)
	for key, value := range safePlatformHeaders(selected.Headers) {
		headers[key] = value
	}
	ticket := MediaTicket{UserID: source.UserID, SourceID: source.ID, Path: p, Kind: "platform-file", VariantID: selected.ID, StreamURL: selected.URL, StreamHeaders: headers, Duration: info.Duration, Container: selected.Ext, IsLive: info.IsLive, VideoCodec: selected.VideoCodec, Width: selected.Width, Height: selected.Height, Bandwidth: int64(selected.Bitrate * 1000)}
	if strings.Contains(selected.Protocol, "m3u8") {
		ticket.Kind = "platform-hls"
	} else if selected.AudioCodec == "none" {
		audio, ok := bestPlatformAudio(info)
		if !ok {
			return MediaTicket{}, errors.New("该画质缺少可用音轨")
		}
		ticket.Kind = "platform-dash"
		ticket.AudioURL = audio.URL
		ticket.AudioCodec = audio.AudioCodec
	}
	return ticket, nil
}

func (m *MediaSourceManager) openPlatform(ctx context.Context, source MediaSource, secret []byte, ticket MediaTicket, method, rangeHeader, ifRange string) (*http.Response, error) {
	if !strings.HasPrefix(ticket.Kind, "platform-") || (method != http.MethodGet && method != http.MethodHead) {
		return nil, errors.New("invalid platform ticket")
	}
	target, err := mediaRemoteURL(ticket.StreamURL)
	if err != nil {
		return nil, err
	}
	var c platformCredentials
	if json.Unmarshal(secret, &c) != nil {
		return nil, errors.New("平台登录数据无效")
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), nil)
	if err != nil {
		return nil, errors.New("平台播放地址无效")
	}
	apply := func(req *http.Request) {
		req.Header.Del("Cookie")
		req.Header.Del("Authorization")
		for key, value := range safePlatformHeaders(ticket.StreamHeaders) {
			req.Header.Set(key, value)
		}
		if req.URL.Scheme == "https" {
			for _, host := range platformDefinitions[source.Type].Hosts[:1] {
				if req.URL.Hostname() == host || strings.HasSuffix(req.URL.Hostname(), "."+host) {
					if c.Cookie != "" {
						req.Header.Set("Cookie", c.Cookie)
					}
					break
				}
			}
		}
	}
	apply(request)
	if rangeHeader != "" {
		request.Header.Set("Range", rangeHeader)
	}
	if ifRange != "" {
		request.Header.Set("If-Range", ifRange)
	}
	client := *m.platformClient
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many redirects")
		}
		if _, err := mediaRemoteURL(next.URL.String()); err != nil {
			return err
		}
		apply(next)
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("平台视频连接失败，请重试或刷新片源")
	}
	return response, nil
}
