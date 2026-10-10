package app

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const webDAVProperties = `<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:"><D:prop>
<D:displayname/><D:resourcetype/><D:getcontentlength/>
<D:getcontenttype/><D:getlastmodified/>
</D:prop></D:propfind>`

type webDAVCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type MediaSourceManager struct {
	repository         Repository
	vault              *CredentialVault
	client             *http.Client
	quarkClient        *http.Client
	platformClient     *http.Client
	platform           *platformResolver
	allowPrivate       bool
	trustedAuthorities map[string]bool
}

func NewMediaSourceManager(
	repository Repository,
	vault *CredentialVault,
	allowPrivate bool,
) *MediaSourceManager {
	manager := &MediaSourceManager{
		repository: repository, vault: vault, allowPrivate: allowPrivate,
	}
	manager.trustedAuthorities = map[string]bool{}
	for _, raw := range strings.Split(os.Getenv("SAMEFRAME_SOURCE_HOST_ALLOWLIST"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" && u.User == nil {
			manager.trustedAuthorities[sourceAuthority(u)] = true
		}
	}
	manager.client = newSourceHTTPClientForAuthorities(allowPrivate, manager.trustedAuthorities)
	manager.quarkClient = newSourceHTTPClient(false)
	manager.platformClient = newSourceHTTPClient(false)
	manager.platform = newPlatformResolver()
	return manager
}

func (m *MediaSourceManager) CreateWebDAV(
	ctx context.Context,
	user User,
	name, baseURL, username, password string,
) (MediaSource, error) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) < 1 || len([]rune(name)) > 64 {
		return MediaSource{}, errors.New("source name must be 1-64 characters")
	}
	parsed, err := m.validateBaseURL(ctx, baseURL)
	if err != nil {
		return MediaSource{}, err
	}
	if len(username) > 256 || len(password) > 2048 {
		return MediaSource{}, errors.New("source credentials are too long")
	}
	sourceID := mustRandomString(12)
	credentials, _ := json.Marshal(webDAVCredentials{Username: username, Password: password})
	encrypted, err := m.vault.Encrypt(credentials, sourceAAD(user.ID, sourceID))
	if err != nil {
		return MediaSource{}, err
	}
	now := time.Now().UnixMilli()
	source := MediaSource{
		ID: sourceID, UserID: user.ID, Type: "webdav", Name: name,
		BaseURL: parsed.String(), CredentialsCiphertext: encrypted,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := m.repository.CreateMediaSource(ctx, source); err != nil {
		return MediaSource{}, err
	}
	return source, nil
}

func (m *MediaSourceManager) List(ctx context.Context, userID string) ([]MediaSource, error) {
	return m.repository.ListMediaSources(ctx, userID)
}

func (m *MediaSourceManager) Delete(ctx context.Context, userID, sourceID string) error {
	if source, secret, err := m.loadSourceSecret(ctx, userID, sourceID); err == nil && source.Type == "emby" {
		logoutContext, cancel := context.WithTimeout(ctx, 2*time.Second)
		m.logoutEmby(logoutContext, source, secret)
		cancel()
	}
	return m.repository.DeleteMediaSource(ctx, userID, sourceID)
}

func (m *MediaSourceManager) Browse(
	ctx context.Context,
	userID, sourceID, requestedPath string,
) ([]MediaFile, error) {
	source, secret, err := m.loadSourceSecret(ctx, userID, sourceID)
	if err != nil {
		return nil, err
	}
	switch source.Type {
	case "webdav":
		credentials, err := decodeWebDAVCredentials(secret)
		if err != nil {
			return nil, err
		}
		return m.browseWebDAV(ctx, source, credentials, requestedPath)
	case "emby":
		return m.browseEmby(ctx, source, secret, requestedPath)
	case "quark":
		return m.browseQuark(ctx, source, secret, requestedPath)
	case "synology", "qnap", "fnos", "nextcloud", "seafile", "truenas":
		return m.browseNAS(ctx, source, secret, requestedPath)
	default:
		if _, ok := platformDefinitions[source.Type]; ok {
			return []MediaFile{}, nil
		}
		return nil, errors.New("unsupported media source type")
	}
}

func (m *MediaSourceManager) browseWebDAV(
	ctx context.Context,
	source MediaSource,
	credentials webDAVCredentials,
	requestedPath string,
) ([]MediaFile, error) {
	target, err := resolveSourcePath(source.BaseURL, requestedPath, true)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(
		ctx, "PROPFIND", target.String(), strings.NewReader(webDAVProperties),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Depth", "1")
	request.Header.Set("Content-Type", "application/xml; charset=utf-8")
	request.SetBasicAuth(credentials.Username, credentials.Password)
	response, err := m.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("WebDAV request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("WebDAV returned HTTP %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, 8<<20)
	var multistatus davMultiStatus
	if err := xml.NewDecoder(limited).Decode(&multistatus); err != nil {
		return nil, fmt.Errorf("invalid WebDAV response: %w", err)
	}
	requestedClean := cleanMediaPath(requestedPath)
	files := make([]MediaFile, 0, len(multistatus.Responses))
	for _, item := range multistatus.Responses {
		file, ok := parseDAVResponse(source.BaseURL, item)
		if !ok || cleanMediaPath(file.Path) == requestedClean {
			continue
		}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDirectory != files[j].IsDirectory {
			return files[i].IsDirectory
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return files, nil
}

func (m *MediaSourceManager) Open(
	ctx context.Context,
	ticket MediaTicket,
	method, rangeHeader, ifRange string,
) (*http.Response, error) {
	source, secret, err := m.loadSourceSecret(ctx, ticket.UserID, ticket.SourceID)
	if err != nil {
		return nil, err
	}
	switch source.Type {
	case "webdav":
		if ticket.Kind != "" && ticket.Kind != "webdav" {
			return nil, errors.New("invalid media ticket")
		}
		credentials, err := decodeWebDAVCredentials(secret)
		if err != nil {
			return nil, err
		}
		return m.openWebDAV(ctx, source, credentials, ticket, method, rangeHeader, ifRange)
	case "emby":
		return m.openEmby(ctx, source, secret, ticket, method, rangeHeader, ifRange)
	case "quark":
		return m.openQuark(ctx, source, secret, ticket, method, rangeHeader, ifRange)
	case "synology", "qnap", "fnos", "nextcloud", "seafile", "truenas":
		return m.openNAS(ctx, source, secret, ticket, method, rangeHeader, ifRange)
	default:
		if _, ok := platformDefinitions[source.Type]; ok {
			return m.openPlatform(ctx, source, secret, ticket, method, rangeHeader, ifRange)
		}
		return nil, errors.New("unsupported media source type")
	}
}

func (m *MediaSourceManager) openWebDAV(
	ctx context.Context,
	source MediaSource,
	credentials webDAVCredentials,
	ticket MediaTicket,
	method, rangeHeader, ifRange string,
) (*http.Response, error) {
	target, err := resolveSourcePath(source.BaseURL, ticket.Path, false)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), nil)
	if err != nil {
		return nil, err
	}
	request.SetBasicAuth(credentials.Username, credentials.Password)
	if rangeHeader != "" {
		request.Header.Set("Range", rangeHeader)
	}
	if ifRange != "" {
		request.Header.Set("If-Range", ifRange)
	}
	return m.client.Do(request)
}

func (m *MediaSourceManager) PrepareMediaTicket(
	ctx context.Context,
	userID, sourceID, requestedPath string,
) (MediaTicket, error) {
	source, secret, err := m.loadSourceSecret(ctx, userID, sourceID)
	if err != nil {
		return MediaTicket{}, err
	}
	switch source.Type {
	case "webdav":
		if _, err := resolveSourcePath(source.BaseURL, requestedPath, false); err != nil {
			return MediaTicket{}, err
		}
		return MediaTicket{
			UserID: userID, SourceID: sourceID, Path: cleanMediaPath(requestedPath), Kind: "webdav",
		}, nil
	case "emby":
		return m.prepareEmbyMediaTicket(ctx, source, secret, userID, requestedPath)
	case "quark":
		return m.prepareQuarkMediaTicket(ctx, source, secret, requestedPath)
	case "synology", "qnap", "fnos", "nextcloud", "seafile", "truenas":
		return m.prepareNASTicket(ctx, source, secret, requestedPath)
	default:
		if _, ok := platformDefinitions[source.Type]; ok {
			return m.preparePlatformTicket(ctx, source, secret, requestedPath, "")
		}
		return MediaTicket{}, errors.New("unsupported media source type")
	}
}

func (m *MediaSourceManager) Subtitles(
	ctx context.Context,
	userID, sourceID, mediaPath string,
) ([]MediaFile, error) {
	source, secret, err := m.loadSourceSecret(ctx, userID, sourceID)
	if err != nil {
		return nil, err
	}
	if _, ok := platformDefinitions[source.Type]; ok {
		return []MediaFile{}, nil
	}
	switch source.Type {
	case "webdav":
		credentials, err := decodeWebDAVCredentials(secret)
		if err != nil {
			return nil, err
		}
		files, err := m.browseWebDAV(ctx, source, credentials, path.Dir(mediaPath))
		if err != nil {
			return nil, err
		}
		subtitles := make([]MediaFile, 0)
		for _, file := range files {
			if !file.IsDirectory && isSubtitlePath(file.Path) {
				subtitles = append(subtitles, file)
			}
		}
		return subtitles, nil
	case "emby":
		return m.embySubtitles(ctx, source, secret, mediaPath)
	case "quark":
		// Embedded subtitle tracks remain available to the player. External
		// subtitle authorization needs provider folder identity, not file names.
		return []MediaFile{}, nil
	case "synology", "qnap", "fnos", "nextcloud", "seafile", "truenas":
		files, err := m.browseNAS(ctx, source, secret, path.Dir(mediaPath))
		if err != nil {
			return nil, err
		}
		result := []MediaFile{}
		for _, file := range files {
			if !file.IsDirectory && isSubtitlePath(file.Name) {
				result = append(result, file)
			}
		}
		return result, nil
	default:
		return nil, errors.New("unsupported media source type")
	}
}

func (m *MediaSourceManager) PrepareSubtitleTicket(
	ctx context.Context,
	userID, sourceID, mediaPath, subtitlePath string,
) (MediaTicket, error) {
	source, secret, err := m.loadSourceSecret(ctx, userID, sourceID)
	if err != nil {
		return MediaTicket{}, err
	}
	switch source.Type {
	case "webdav":
		clean := cleanMediaPath(subtitlePath)
		if !isSubtitlePath(clean) || path.Dir(clean) != path.Dir(cleanMediaPath(mediaPath)) {
			return MediaTicket{}, ErrForbidden
		}
		if _, err := resolveSourcePath(source.BaseURL, clean, false); err != nil {
			return MediaTicket{}, err
		}
		return MediaTicket{
			UserID: userID, SourceID: sourceID, Path: clean, Kind: "webdav",
		}, nil
	case "emby":
		return m.prepareEmbySubtitleTicket(ctx, source, secret, userID, mediaPath, subtitlePath)
	case "synology", "qnap", "fnos", "nextcloud", "seafile", "truenas":
		if path.Dir(cleanMediaPath(mediaPath)) != path.Dir(cleanMediaPath(subtitlePath)) || !isSubtitlePath(subtitlePath) {
			return MediaTicket{}, ErrForbidden
		}
		return m.prepareNASTicket(ctx, source, secret, subtitlePath)
	default:
		return MediaTicket{}, errors.New("unsupported media source type")
	}
}

func (m *MediaSourceManager) loadSourceSecret(
	ctx context.Context,
	userID, sourceID string,
) (MediaSource, []byte, error) {
	source, err := m.repository.GetMediaSource(ctx, userID, sourceID)
	if err != nil {
		return MediaSource{}, nil, err
	}
	plaintext, err := m.vault.Decrypt(
		source.CredentialsCiphertext, sourceAAD(userID, sourceID),
	)
	if err != nil {
		return MediaSource{}, nil, err
	}
	return source, plaintext, nil
}

func decodeWebDAVCredentials(plaintext []byte) (webDAVCredentials, error) {
	var credentials webDAVCredentials
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return webDAVCredentials{}, errors.New("invalid stored credentials")
	}
	return credentials, nil
}

func sourceAAD(userID, sourceID string) string { return userID + ":" + sourceID }

func validateSourceBaseURL(
	ctx context.Context,
	raw string,
	allowPrivate bool,
) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return nil, errors.New("source URL must be absolute")
	}
	if parsed.Scheme != "https" && !(allowPrivate && parsed.Scheme == "http") {
		return nil, errors.New("source URL must use HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("source URL must not contain credentials, query, or fragment")
	}
	if err := validateSourceHost(ctx, parsed.Hostname(), allowPrivate); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return parsed, nil
}

func validateSourceHost(ctx context.Context, hostname string, allowPrivate bool) error {
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil || len(addresses) == 0 {
		return errors.New("source hostname could not be resolved")
	}
	if allowPrivate {
		return nil
	}
	for _, address := range addresses {
		if disallowedSourceIP(address.IP) {
			return errors.New("source resolves to a private or reserved address")
		}
	}
	return nil
}

func disallowedSourceIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	address = address.Unmap()
	if address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() ||
		address.IsUnspecified() || address.IsMulticast() {
		return true
	}
	cgnat := netip.MustParsePrefix("100.64.0.0/10")
	return cgnat.Contains(address)
}

func newSourceHTTPClient(allowPrivate bool) *http.Client {
	return newSourceHTTPClientForAuthorities(allowPrivate, nil)
}

func (m *MediaSourceManager) validateBaseURL(ctx context.Context, raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, errors.New("source URL must be absolute")
	}
	return validateSourceBaseURL(ctx, raw, m.allowPrivate || m.trustedAuthorities[sourceAuthority(u)])
}

func newSourceHTTPClientForAuthorities(allowPrivate bool, authorities map[string]bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, candidate := range addresses {
				if !allowPrivate && !authorities[strings.ToLower(host)+":"+port] && disallowedSourceIP(candidate.IP) {
					continue
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
			}
			return nil, errors.New("source has no permitted address")
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		IdleConnTimeout:       60 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   0,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many source redirects")
			}
			if len(via) > 0 && sourceAuthority(request.URL) != sourceAuthority(via[0].URL) {
				return errors.New("cross-host source redirect blocked")
			}
			if request.URL.User != nil ||
				(request.URL.Scheme != "https" && !((allowPrivate || authorities[sourceAuthority(request.URL)]) && request.URL.Scheme == "http")) {
				return errors.New("unsafe source redirect blocked")
			}
			if len(via) > 0 && via[0].URL.Scheme == "https" && request.URL.Scheme != "https" {
				return errors.New("source TLS downgrade blocked")
			}
			return validateSourceHost(request.Context(), request.URL.Hostname(), allowPrivate || authorities[sourceAuthority(request.URL)])
		},
	}
}

func sourceAuthority(value *url.URL) string {
	port := value.Port()
	if port == "" {
		switch strings.ToLower(value.Scheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return strings.ToLower(value.Hostname()) + ":" + port
}

func resolveSourcePath(baseURL, requestedPath string, directory bool) (*url.URL, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	clean := cleanMediaPath(requestedPath)
	for _, segment := range strings.Split(strings.ReplaceAll(requestedPath, "\\", "/"), "/") {
		if segment == ".." {
			return nil, errors.New("parent path traversal is not allowed")
		}
	}
	segments := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	safeSegments := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment != "" && segment != "." {
			safeSegments = append(safeSegments, segment)
		}
	}
	base.RawPath = ""
	base.Path = path.Join(base.Path, strings.Join(safeSegments, "/"))
	if directory && !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	return base, nil
}

func cleanMediaPath(value string) string {
	return "/" + strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(value, "\\", "/")), "/")
}

func isSubtitlePath(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".srt", ".vtt", ".ass", ".ssa":
		return true
	default:
		return false
	}
}

type davMultiStatus struct {
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href      string        `xml:"href"`
	PropStats []davPropStat `xml:"propstat"`
}

type davPropStat struct {
	Status string  `xml:"status"`
	Prop   davProp `xml:"prop"`
}

type davProp struct {
	DisplayName   string          `xml:"displayname"`
	ContentLength string          `xml:"getcontentlength"`
	ContentType   string          `xml:"getcontenttype"`
	LastModified  string          `xml:"getlastmodified"`
	ResourceType  davResourceType `xml:"resourcetype"`
}

type davResourceType struct {
	Collection *struct{} `xml:"collection"`
}

func parseDAVResponse(baseURL string, response davResponse) (MediaFile, bool) {
	var property *davProp
	for index := range response.PropStats {
		if strings.Contains(response.PropStats[index].Status, " 200 ") {
			property = &response.PropStats[index].Prop
			break
		}
	}
	if property == nil {
		return MediaFile{}, false
	}
	href, err := url.Parse(response.Href)
	if err != nil {
		return MediaFile{}, false
	}
	decodedPath, err := url.PathUnescape(href.Path)
	if err != nil {
		return MediaFile{}, false
	}
	base, _ := url.Parse(baseURL)
	basePath := strings.TrimSuffix(base.Path, "/")
	if decodedPath != basePath && !strings.HasPrefix(decodedPath, basePath+"/") {
		return MediaFile{}, false
	}
	relative := strings.TrimPrefix(decodedPath, basePath)
	mediaPath := cleanMediaPath(relative)
	name := strings.TrimSpace(property.DisplayName)
	if name == "" {
		name = path.Base(strings.TrimSuffix(decodedPath, "/"))
	}
	size, _ := strconv.ParseInt(strings.TrimSpace(property.ContentLength), 10, 64)
	if size < 0 {
		size = 0
	}
	return MediaFile{
		Name: name, Path: mediaPath, IsDirectory: property.ResourceType.Collection != nil,
		Size: size, ContentType: property.ContentType, ModifiedAt: property.LastModified,
	}, true
}
