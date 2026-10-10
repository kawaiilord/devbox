package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	quarkAPI           = "https://drive.quark.cn/1/clouddrive"
	quarkWebsite       = "https://pan.quark.cn/"
	quarkUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) quark-cloud-drive/2.5.20 Chrome/100.0.4896.160 Electron/18.3.5.4-b478491100 Safari/537.36 Channel/pckk_other_ch"
	quarkResponseLimit = 8 << 20
	quarkPageSize      = 100
	quarkMaxFiles      = 5000
)

var quarkFileIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var errQuarkLogin = errors.New("夸克登录状态已失效，请在夸克网页登录后更新 Cookie")

type quarkCredentials struct {
	Cookie string `json:"cookie"`
}

type quarkResponse struct {
	Code     *int            `json:"code"`
	Status   int             `json:"status"`
	Data     json.RawMessage `json:"data"`
	Metadata struct {
		Total int `json:"_total"`
	} `json:"metadata"`
}

type quarkFile struct {
	ID          string `json:"fid"`
	Name        string `json:"file_name"`
	File        bool   `json:"file"`
	Category    int    `json:"category"`
	Size        int64  `json:"size"`
	ContentType string `json:"format_type"`
	UpdatedAt   int64  `json:"updated_at"`
	DownloadURL string `json:"download_url"`
}

func normalizeQuarkCookie(raw string) (string, error) {
	if len(raw) > 16384 || strings.ContainsAny(raw, "\r\n\x00") {
		return "", errors.New("夸克 Cookie 格式无效，请复制完整的 Cookie 请求头值")
	}
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(raw), "cookie:") {
		raw = strings.TrimSpace(raw[7:])
	}
	cookies, err := http.ParseCookie(raw)
	if err != nil || len(cookies) == 0 {
		return "", errors.New("夸克 Cookie 格式无效")
	}
	seen := map[string]bool{}
	parts := make([]string, 0, len(cookies))
	hasSession := false
	for _, cookie := range cookies {
		if cookie.Valid() != nil || seen[cookie.Name] {
			return "", errors.New("夸克 Cookie 格式无效")
		}
		seen[cookie.Name] = true
		parts = append(parts, cookie.String())
		if (cookie.Name == "__pus" || cookie.Name == "__puus") && cookie.Value != "" {
			hasSession = true
		}
	}
	if !hasSession {
		return "", errors.New("未找到夸克登录凭据，请登录后复制完整 Cookie")
	}
	return strings.Join(parts, "; "), nil
}

func decodeQuarkCredentials(secret []byte) (quarkCredentials, error) {
	var credentials quarkCredentials
	if json.Unmarshal(secret, &credentials) != nil {
		return credentials, errors.New("invalid stored Quark credentials")
	}
	cookie, err := normalizeQuarkCookie(credentials.Cookie)
	if err != nil {
		return quarkCredentials{}, errors.New("invalid stored Quark credentials")
	}
	credentials.Cookie = cookie
	return credentials, nil
}

// SaveQuark verifies a read-only directory request before persisting the login.
// An existing ID updates that owner's source in place, preserving room references.
func (m *MediaSourceManager) SaveQuark(ctx context.Context, user User, sourceID, name, rawCookie string) (MediaSource, error) {
	var source MediaSource
	var err error
	if sourceID != "" {
		source, err = m.repository.GetMediaSource(ctx, user.ID, sourceID)
		if err != nil {
			return MediaSource{}, err
		}
		if source.Type != "quark" {
			return MediaSource{}, ErrForbidden
		}
	} else {
		name = strings.TrimSpace(name)
		if len([]rune(name)) < 1 || len([]rune(name)) > 64 {
			return MediaSource{}, errors.New("名称需为 1–64 个字符")
		}
	}
	cookie, err := normalizeQuarkCookie(rawCookie)
	if err != nil {
		return MediaSource{}, err
	}
	credentials := quarkCredentials{Cookie: cookie}
	// Do not update an existing stored credential until validation has succeeded.
	probe := MediaSource{}
	response, err := m.quarkJSON(ctx, &probe, &credentials, http.MethodGet, "/file/sort", url.Values{
		"pdir_fid": {"0"}, "_page": {"1"}, "_size": {"1"},
	}, nil)
	if err != nil {
		return MediaSource{}, err
	}
	var listing struct {
		List []quarkFile `json:"list"`
	}
	if json.Unmarshal(response.Data, &listing) != nil || listing.List == nil {
		return MediaSource{}, errors.New("夸克目录响应无效，请稍后重试")
	}
	if sourceID == "" {
		now := time.Now().UnixMilli()
		source = MediaSource{ID: mustRandomString(12), UserID: user.ID, Type: "quark", Name: name,
			BaseURL: quarkWebsite, CreatedAt: now, UpdatedAt: now}
	}
	secret, _ := json.Marshal(credentials)
	encrypted, err := m.vault.Encrypt(secret, sourceAAD(user.ID, source.ID))
	if err != nil {
		return MediaSource{}, err
	}
	if sourceID == "" {
		source.CredentialsCiphertext = encrypted
		err = m.repository.CreateMediaSource(ctx, source)
	} else {
		var replaced bool
		replaced, err = m.repository.ReplaceMediaSourceCredentials(ctx, user.ID, source.ID, source.CredentialsCiphertext, encrypted)
		if err == nil && !replaced {
			err = errors.New("登录状态刚刚更新，请重试")
		}
		source.CredentialsCiphertext = encrypted
		source.UpdatedAt = time.Now().UnixMilli()
	}
	return source, err
}

func (m *MediaSourceManager) quarkJSON(ctx context.Context, source *MediaSource, credentials *quarkCredentials, method, route string, query url.Values, body any) (quarkResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if query == nil {
		query = url.Values{}
	}
	query.Set("pr", "ucpro")
	query.Set("fr", "pc")
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return quarkResponse{}, errors.New("invalid Quark request")
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, quarkAPI+route+"?"+query.Encode(), reader)
	if err != nil {
		return quarkResponse{}, errors.New("invalid Quark request")
	}
	request.Header.Set("Cookie", credentials.Cookie)
	request.Header.Set("Referer", quarkWebsite)
	request.Header.Set("User-Agent", quarkUserAgent)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	// API requests never follow redirects, including redirects within quark.cn.
	client := *m.quarkClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return quarkResponse{}, errors.New("连接夸克失败，请稍后重试")
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return quarkResponse{}, errQuarkLogin
	}
	if response.StatusCode != http.StatusOK {
		return quarkResponse{}, fmt.Errorf("夸克暂时不可用（HTTP %d）", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, quarkResponseLimit+1))
	var result quarkResponse
	if err != nil || len(data) > quarkResponseLimit || json.Unmarshal(data, &result) != nil || result.Code == nil {
		return quarkResponse{}, errors.New("夸克响应无效，请稍后重试")
	}
	if result.Status == 401 || result.Status == 403 || (*result.Code >= 31001 && *result.Code <= 31005) {
		return quarkResponse{}, errQuarkLogin
	}
	if *result.Code != 0 || result.Status >= 400 {
		// Provider messages can include account data. Return only the numeric code.
		return quarkResponse{}, fmt.Errorf("夸克请求未成功（%d），请检查账号状态后重试", *result.Code)
	}
	if err := m.rotateQuarkCookie(ctx, source, credentials, response.Cookies()); err != nil {
		return quarkResponse{}, err
	}
	return result, nil
}

func (m *MediaSourceManager) rotateQuarkCookie(ctx context.Context, source *MediaSource, credentials *quarkCredentials, updates []*http.Cookie) error {
	cookies, _ := http.ParseCookie(credentials.Cookie)
	changed := false
	for _, update := range updates {
		if (update.Name != "__puus" && update.Name != "__pus") || update.Valid() != nil {
			continue
		}
		if update.MaxAge < 0 || update.Value == "" {
			return errQuarkLogin
		}
		found := false
		for _, cookie := range cookies {
			if cookie.Name == update.Name {
				found = true
				if cookie.Value != update.Value {
					cookie.Value = update.Value
					changed = true
				}
			}
		}
		if !found {
			cookies = append(cookies, &http.Cookie{Name: update.Name, Value: update.Value})
			changed = true
		}
	}
	if !changed {
		return nil
	}
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		parts = append(parts, cookie.String())
	}
	next, err := normalizeQuarkCookie(strings.Join(parts, "; "))
	if err != nil {
		return errQuarkLogin
	}
	credentials.Cookie = next
	if source.ID == "" {
		return nil
	}
	secret, _ := json.Marshal(credentials)
	encrypted, err := m.vault.Encrypt(secret, sourceAAD(source.UserID, source.ID))
	if err != nil {
		return err
	}
	replaced, err := m.repository.ReplaceMediaSourceCredentials(ctx, source.UserID, source.ID, source.CredentialsCiphertext, encrypted)
	if err != nil {
		return errors.New("无法保存夸克登录状态，请重试")
	}
	if replaced {
		source.CredentialsCiphertext = encrypted
		return nil
	}
	// Another request or a manual login won the comparison. Use its credentials.
	current, secret, err := m.loadSourceSecret(ctx, source.UserID, source.ID)
	if err != nil {
		return err
	}
	*credentials, err = decodeQuarkCredentials(secret)
	*source = current
	return err
}

func quarkIDFromPath(value string, rootAllowed bool) (string, error) {
	if value == "" || value == "/" {
		if rootAllowed {
			return "0", nil
		}
		return "", errors.New("请选择网盘中的视频文件")
	}
	if len(value) > 4096 || !strings.HasPrefix(value, "/") {
		return "", errors.New("invalid Quark file path")
	}
	segments := strings.Split(strings.Trim(value, "/"), "/")
	for _, segment := range segments {
		if !quarkFileIDPattern.MatchString(segment) || segment == "0" {
			return "", errors.New("invalid Quark file path")
		}
	}
	return segments[len(segments)-1], nil
}

func quarkIsVideo(file quarkFile) bool {
	if file.Category == 1 || strings.HasPrefix(strings.ToLower(file.ContentType), "video/") {
		return true
	}
	switch strings.ToLower(path.Ext(file.Name)) {
	case ".mp4", ".mkv", ".webm", ".mov", ".avi", ".m4v", ".ts", ".m2ts", ".mpg", ".mpeg", ".wmv", ".flv":
		return true
	}
	return false
}

func (m *MediaSourceManager) browseQuark(ctx context.Context, source MediaSource, secret []byte, requestedPath string) ([]MediaFile, error) {
	parentID, err := quarkIDFromPath(requestedPath, true)
	if err != nil {
		return nil, err
	}
	credentials, err := decodeQuarkCredentials(secret)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	files := make([]MediaFile, 0)
	seen := map[string]bool{}
	for page := 1; ; page++ {
		result, err := m.quarkJSON(ctx, &source, &credentials, http.MethodGet, "/file/sort", url.Values{
			"pdir_fid": {parentID}, "_page": {strconv.Itoa(page)}, "_size": {strconv.Itoa(quarkPageSize)},
			"_fetch_total": {"1"}, "_sort": {"file_type:asc,file_name:asc"},
		}, nil)
		if err != nil {
			return nil, err
		}
		var listing struct {
			List []quarkFile `json:"list"`
		}
		if json.Unmarshal(result.Data, &listing) != nil || listing.List == nil {
			return nil, errors.New("夸克目录响应无效")
		}
		for _, file := range listing.List {
			if !quarkFileIDPattern.MatchString(file.ID) || file.ID == "0" || seen[file.ID] {
				continue
			}
			seen[file.ID] = true
			if file.File && !quarkIsVideo(file) {
				continue
			}
			files = append(files, MediaFile{Name: sanitizeAuditText(html.UnescapeString(file.Name), 256),
				Path: path.Join("/", requestedPath, file.ID), IsDirectory: !file.File,
				Size: max(file.Size, 0), ContentType: sanitizeAuditText(file.ContentType, 128),
				ModifiedAt: time.UnixMilli(file.UpdatedAt).UTC().Format(time.RFC3339)})
		}
		if (result.Metadata.Total > 0 && page*quarkPageSize >= result.Metadata.Total) || len(listing.List) < quarkPageSize {
			break
		}
		if page*quarkPageSize >= quarkMaxFiles {
			return nil, errors.New("目录文件过多，请在夸克中整理到子文件夹后重试")
		}
	}
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].IsDirectory != files[j].IsDirectory {
			return files[i].IsDirectory
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return files, nil
}

// Download links are requested on demand; neither rooms nor Redis tickets store
// signed upstream URLs. Each new Range request can outlive the previous link.
func (m *MediaSourceManager) quarkDownload(ctx context.Context, source *MediaSource, credentials *quarkCredentials, fileID string) (*url.URL, error) {
	result, err := m.quarkJSON(ctx, source, credentials, http.MethodPost, "/file/download", nil, map[string]any{"fids": []string{fileID}})
	if err != nil {
		return nil, err
	}
	var files []quarkFile
	if json.Unmarshal(result.Data, &files) != nil || len(files) != 1 || files[0].ID != fileID || !quarkIsVideo(files[0]) {
		return nil, errors.New("夸克未返回可播放的视频，请重新选片")
	}
	return quarkDownloadURL(files[0].DownloadURL)
}

func quarkDownloadURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" ||
		(parsed.Port() != "" && parsed.Port() != "443") || net.ParseIP(parsed.Hostname()) != nil {
		return nil, errors.New("夸克返回了不安全的播放地址")
	}
	return parsed, nil
}

func (m *MediaSourceManager) prepareQuarkMediaTicket(ctx context.Context, source MediaSource, secret []byte, requestedPath string) (MediaTicket, error) {
	fileID, err := quarkIDFromPath(requestedPath, false)
	if err != nil {
		return MediaTicket{}, err
	}
	credentials, err := decodeQuarkCredentials(secret)
	if err != nil {
		return MediaTicket{}, err
	}
	if _, err := m.quarkDownload(ctx, &source, &credentials, fileID); err != nil {
		return MediaTicket{}, err
	}
	return MediaTicket{UserID: source.UserID, SourceID: source.ID, Path: cleanMediaPath(requestedPath), Kind: "quark-video", ItemID: fileID}, nil
}

func (m *MediaSourceManager) openQuark(ctx context.Context, source MediaSource, secret []byte, ticket MediaTicket, method, rangeHeader, ifRange string) (*http.Response, error) {
	fileID, err := quarkIDFromPath(ticket.Path, false)
	if err != nil || ticket.Kind != "quark-video" || ticket.ItemID != fileID || (method != http.MethodGet && method != http.MethodHead) {
		return nil, errors.New("invalid Quark media ticket")
	}
	credentials, err := decodeQuarkCredentials(secret)
	if err != nil {
		return nil, err
	}
	client := *m.quarkClient
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many Quark media redirects")
		}
		if _, err := quarkDownloadURL(request.URL.String()); err != nil {
			return err
		}
		// The transport still validates DNS at every connection. Cookies are
		// scoped explicitly: signed third-party CDN links never receive them.
		setQuarkStreamHeaders(request, credentials.Cookie)
		return nil
	}
	for attempt := 0; attempt < 2; attempt++ {
		target, err := m.quarkDownload(ctx, &source, &credentials, fileID)
		if err != nil {
			return nil, err
		}
		request, err := http.NewRequestWithContext(ctx, method, target.String(), nil)
		if err != nil {
			return nil, errors.New("invalid Quark playback request")
		}
		setQuarkStreamHeaders(request, credentials.Cookie)
		if rangeHeader != "" {
			request.Header.Set("Range", rangeHeader)
		}
		if ifRange != "" {
			request.Header.Set("If-Range", ifRange)
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, errors.New("夸克视频连接失败，请稍后重试")
		}
		if attempt == 0 && (response.StatusCode == 401 || response.StatusCode == 403) {
			response.Body.Close()
			continue
		}
		return response, nil
	}
	return nil, errors.New("夸克视频暂时不可用")
}

func setQuarkStreamHeaders(request *http.Request, cookie string) {
	request.Header.Del("Cookie")
	request.Header.Del("Authorization")
	host := strings.ToLower(request.URL.Hostname())
	if host == "quark.cn" || strings.HasSuffix(host, ".quark.cn") {
		request.Header.Set("Cookie", cookie)
	}
	request.Header.Set("Referer", quarkWebsite)
	request.Header.Set("User-Agent", quarkUserAgent)
}
