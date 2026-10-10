package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	embyClientName    = "SameFrame"
	embyClientVersion = "1.0.0"
	embyResponseLimit = 8 << 20
)

var embyIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
var embyContainerPattern = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

type embyCredentials struct {
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
	DeviceID    string `json:"device_id"`
	ServerID    string `json:"server_id,omitempty"`
}

type embyAuthenticationResult struct {
	AccessToken string `json:"AccessToken"`
	ServerID    string `json:"ServerId"`
	User        struct {
		ID string `json:"Id"`
	} `json:"User"`
}

type embyItemsResponse struct {
	Items            []embyItem `json:"Items"`
	TotalRecordCount int        `json:"TotalRecordCount"`
}

type embyItem struct {
	ID           string            `json:"Id"`
	Name         string            `json:"Name"`
	Type         string            `json:"Type"`
	MediaType    string            `json:"MediaType"`
	IsFolder     bool              `json:"IsFolder"`
	Container    string            `json:"Container"`
	DateCreated  string            `json:"DateCreated"`
	MediaSources []embyMediaSource `json:"MediaSources"`
	MediaStreams []embyMediaStream `json:"MediaStreams"`
}

type embyMediaSource struct {
	ID                   string            `json:"Id"`
	Container            string            `json:"Container"`
	Size                 int64             `json:"Size"`
	SupportsDirectPlay   bool              `json:"SupportsDirectPlay"`
	SupportsDirectStream bool              `json:"SupportsDirectStream"`
	MediaStreams         []embyMediaStream `json:"MediaStreams"`
}

type embyMediaStream struct {
	Index                int    `json:"Index"`
	Type                 string `json:"Type"`
	Codec                string `json:"Codec"`
	Language             string `json:"Language"`
	DisplayTitle         string `json:"DisplayTitle"`
	IsTextSubtitleStream bool   `json:"IsTextSubtitleStream"`
}

func (m *MediaSourceManager) CreateEmby(
	ctx context.Context,
	user User,
	name, baseURL, username, password string,
) (MediaSource, error) {
	name = strings.TrimSpace(name)
	username = strings.TrimSpace(username)
	if len([]rune(name)) < 1 || len([]rune(name)) > 64 {
		return MediaSource{}, errors.New("source name must be 1-64 characters")
	}
	if username == "" || len([]rune(username)) > 256 || len(password) > 2048 {
		return MediaSource{}, errors.New("invalid Emby credentials")
	}
	parsed, err := m.validateBaseURL(ctx, baseURL)
	if err != nil {
		return MediaSource{}, err
	}
	sourceID := mustRandomString(12)
	credentials := embyCredentials{DeviceID: "sameframe-" + sourceID}
	payload, _ := json.Marshal(map[string]string{"Username": username, "Pw": password})
	request, err := m.newEmbyRequest(
		ctx,
		parsed.String(),
		credentials,
		http.MethodPost,
		[]string{"Users", "AuthenticateByName"},
		nil,
		bytes.NewReader(payload),
	)
	if err != nil {
		return MediaSource{}, err
	}
	response, err := m.client.Do(request)
	if err != nil {
		return MediaSource{}, errors.New("Emby server is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return MediaSource{}, fmt.Errorf("Emby authentication returned HTTP %d", response.StatusCode)
	}
	var authentication embyAuthenticationResult
	if err := decodeEmbyJSON(response.Body, &authentication); err != nil {
		return MediaSource{}, err
	}
	if !validEmbyIdentifier(authentication.User.ID) ||
		!validEmbyToken(authentication.AccessToken) {
		return MediaSource{}, errors.New("Emby authentication response is invalid")
	}
	credentials.UserID = authentication.User.ID
	credentials.AccessToken = authentication.AccessToken
	credentials.ServerID = sanitizeAuditText(authentication.ServerID, 128)
	secret, _ := json.Marshal(credentials)
	encrypted, err := m.vault.Encrypt(secret, sourceAAD(user.ID, sourceID))
	if err != nil {
		return MediaSource{}, err
	}
	now := time.Now().UnixMilli()
	source := MediaSource{
		ID: sourceID, UserID: user.ID, Type: "emby", Name: name,
		BaseURL: parsed.String(), CredentialsCiphertext: encrypted,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := m.repository.CreateMediaSource(ctx, source); err != nil {
		return MediaSource{}, err
	}
	return source, nil
}

func (m *MediaSourceManager) browseEmby(
	ctx context.Context,
	source MediaSource,
	secret []byte,
	requestedPath string,
) ([]MediaFile, error) {
	credentials, err := decodeEmbyCredentials(secret)
	if err != nil {
		return nil, err
	}
	parentID, err := embyItemIDFromPath(requestedPath, true)
	if err != nil {
		return nil, err
	}
	query := url.Values{
		"Fields":    {"MediaSources,MediaStreams,DateCreated"},
		"SortBy":    {"SortName"},
		"SortOrder": {"Ascending"},
		"Limit":     {"200"},
	}
	if parentID != "" {
		query.Set("ParentId", parentID)
	}
	var result embyItemsResponse
	if err := m.doEmbyJSON(
		ctx, source, credentials, http.MethodGet,
		[]string{"Users", credentials.UserID, "Items"}, query, nil, &result,
	); err != nil {
		return nil, err
	}
	files := make([]MediaFile, 0, len(result.Items))
	for _, item := range result.Items {
		if len(files) >= 200 {
			break
		}
		if !validEmbyIdentifier(item.ID) || (!item.IsFolder && !embyItemIsVideo(item)) {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = item.ID
		}
		name = sanitizeAuditText(name, 256)
		size := int64(0)
		container := item.Container
		if len(item.MediaSources) > 0 {
			size = item.MediaSources[0].Size
			if container == "" {
				container = item.MediaSources[0].Container
			}
		}
		if size < 0 {
			size = 0
		}
		files = append(files, MediaFile{
			Name: name, Path: appendEmbyItemPath(requestedPath, item.ID),
			IsDirectory: item.IsFolder, Size: size,
			ContentType: embyContentType(container), ModifiedAt: sanitizeAuditText(item.DateCreated, 128),
		})
	}
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].IsDirectory != files[j].IsDirectory {
			return files[i].IsDirectory
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return files, nil
}

func (m *MediaSourceManager) prepareEmbyMediaTicket(
	ctx context.Context,
	source MediaSource,
	secret []byte,
	userID, requestedPath string,
) (MediaTicket, error) {
	_, item, mediaSource, err := m.embyPlaybackItem(ctx, source, secret, requestedPath)
	if err != nil {
		return MediaTicket{}, err
	}
	container := normalizeEmbyContainer(mediaSource.Container)
	if container == "" {
		container = normalizeEmbyContainer(item.Container)
	}
	if container == "" {
		return MediaTicket{}, errors.New("Emby media container is unsupported")
	}
	return MediaTicket{
		UserID: userID, SourceID: source.ID, Path: cleanMediaPath(requestedPath),
		Kind: "emby-video", ItemID: item.ID, MediaSourceID: mediaSource.ID,
		Container: container, PlaySessionID: mustRandomString(18),
	}, nil
}

func (m *MediaSourceManager) embySubtitles(
	ctx context.Context,
	source MediaSource,
	secret []byte,
	mediaPath string,
) ([]MediaFile, error) {
	_, _, mediaSource, err := m.embyPlaybackItem(ctx, source, secret, mediaPath)
	if err != nil {
		return nil, err
	}
	streams := mediaSource.MediaStreams
	subtitles := make([]MediaFile, 0)
	for _, stream := range streams {
		if !strings.EqualFold(stream.Type, "Subtitle") || !stream.IsTextSubtitleStream || stream.Index < 0 {
			continue
		}
		name := strings.TrimSpace(stream.DisplayTitle)
		if name == "" {
			name = strings.TrimSpace(stream.Language)
		}
		if name == "" {
			name = "Subtitle " + strconv.Itoa(stream.Index)
		}
		subtitles = append(subtitles, MediaFile{
			Name:        sanitizeAuditText(name, 256),
			Path:        embySubtitlePath(mediaPath, stream.Index, "vtt"),
			ContentType: "text/vtt",
		})
	}
	return subtitles, nil
}

func (m *MediaSourceManager) prepareEmbySubtitleTicket(
	ctx context.Context,
	source MediaSource,
	secret []byte,
	userID, mediaPath, subtitlePath string,
) (MediaTicket, error) {
	index, format, err := parseEmbySubtitlePath(mediaPath, subtitlePath)
	if err != nil {
		return MediaTicket{}, err
	}
	_, item, mediaSource, err := m.embyPlaybackItem(ctx, source, secret, mediaPath)
	if err != nil {
		return MediaTicket{}, err
	}
	found := false
	for _, stream := range mediaSource.MediaStreams {
		if stream.Index == index && strings.EqualFold(stream.Type, "Subtitle") && stream.IsTextSubtitleStream {
			found = true
			break
		}
	}
	if !found {
		return MediaTicket{}, ErrForbidden
	}
	return MediaTicket{
		UserID: userID, SourceID: source.ID, Path: cleanMediaPath(subtitlePath),
		Kind: "emby-subtitle", ItemID: item.ID, MediaSourceID: mediaSource.ID,
		PlaySessionID: mustRandomString(18), SubtitleIndex: index, SubtitleFormat: format,
	}, nil
}

func (m *MediaSourceManager) openEmby(
	ctx context.Context,
	source MediaSource,
	secret []byte,
	ticket MediaTicket,
	method, rangeHeader, ifRange string,
) (*http.Response, error) {
	credentials, err := decodeEmbyCredentials(secret)
	if err != nil {
		return nil, err
	}
	var segments []string
	query := url.Values{}
	switch ticket.Kind {
	case "emby-video":
		if !validEmbyIdentifier(ticket.ItemID) || !validEmbyIdentifier(ticket.MediaSourceID) ||
			normalizeEmbyContainer(ticket.Container) != ticket.Container || ticket.PlaySessionID == "" {
			return nil, errors.New("invalid Emby media ticket")
		}
		segments = []string{"Videos", ticket.ItemID, "stream." + ticket.Container}
		query.Set("Static", "true")
		query.Set("MediaSourceId", ticket.MediaSourceID)
		query.Set("PlaySessionId", ticket.PlaySessionID)
		query.Set("DeviceId", credentials.DeviceID)
	case "emby-subtitle":
		if !validEmbyIdentifier(ticket.ItemID) || !validEmbyIdentifier(ticket.MediaSourceID) ||
			ticket.SubtitleIndex < 0 || ticket.SubtitleFormat != "vtt" {
			return nil, errors.New("invalid Emby subtitle ticket")
		}
		segments = []string{
			"Videos", ticket.ItemID, ticket.MediaSourceID, "Subtitles",
			strconv.Itoa(ticket.SubtitleIndex), "Stream." + ticket.SubtitleFormat,
		}
	default:
		return nil, errors.New("invalid Emby media ticket")
	}
	request, err := m.newEmbyRequest(
		ctx, source.BaseURL, credentials, method, segments, query, nil,
	)
	if err != nil {
		return nil, err
	}
	if ticket.Kind == "emby-subtitle" {
		request.Header.Set("Accept", "text/vtt")
	} else {
		request.Header.Set("Accept", "*/*")
	}
	if rangeHeader != "" {
		request.Header.Set("Range", rangeHeader)
	}
	if ifRange != "" {
		request.Header.Set("If-Range", ifRange)
	}
	return m.client.Do(request)
}

func (m *MediaSourceManager) embyPlaybackItem(
	ctx context.Context,
	source MediaSource,
	secret []byte,
	requestedPath string,
) (embyCredentials, embyItem, embyMediaSource, error) {
	credentials, err := decodeEmbyCredentials(secret)
	if err != nil {
		return embyCredentials{}, embyItem{}, embyMediaSource{}, err
	}
	itemID, err := embyItemIDFromPath(requestedPath, false)
	if err != nil {
		return embyCredentials{}, embyItem{}, embyMediaSource{}, err
	}
	var item embyItem
	if err := m.doEmbyJSON(
		ctx, source, credentials, http.MethodGet,
		[]string{"Users", credentials.UserID, "Items", itemID}, nil, nil, &item,
	); err != nil {
		return embyCredentials{}, embyItem{}, embyMediaSource{}, err
	}
	if item.ID != itemID || item.IsFolder || !embyItemIsVideo(item) {
		return embyCredentials{}, embyItem{}, embyMediaSource{}, errors.New("Emby item is not a playable video")
	}
	var fallback *embyMediaSource
	for index := range item.MediaSources {
		candidate := item.MediaSources[index]
		if !validEmbyIdentifier(candidate.ID) {
			continue
		}
		if len(candidate.MediaStreams) == 0 {
			candidate.MediaStreams = item.MediaStreams
		}
		if fallback == nil {
			copy := candidate
			fallback = &copy
		}
		if candidate.SupportsDirectPlay || candidate.SupportsDirectStream {
			return credentials, item, candidate, nil
		}
	}
	if fallback != nil {
		return credentials, item, *fallback, nil
	}
	return embyCredentials{}, embyItem{}, embyMediaSource{}, errors.New("Emby item has no playable media source")
}

func (m *MediaSourceManager) doEmbyJSON(
	ctx context.Context,
	source MediaSource,
	credentials embyCredentials,
	method string,
	segments []string,
	query url.Values,
	body io.Reader,
	target any,
) error {
	request, err := m.newEmbyRequest(
		ctx, source.BaseURL, credentials, method, segments, query, body,
	)
	if err != nil {
		return err
	}
	response, err := m.client.Do(request)
	if err != nil {
		return errors.New("Emby server is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Emby returned HTTP %d", response.StatusCode)
	}
	return decodeEmbyJSON(response.Body, target)
}

func (m *MediaSourceManager) newEmbyRequest(
	ctx context.Context,
	baseURL string,
	credentials embyCredentials,
	method string,
	segments []string,
	query url.Values,
	body io.Reader,
) (*http.Request, error) {
	target, err := embyEndpoint(baseURL, segments...)
	if err != nil {
		return nil, err
	}
	target.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	authorization := fmt.Sprintf(
		`Emby UserId="%s", Client="%s", Device="SameFrame Server", DeviceId="%s", Version="%s"`,
		credentials.UserID, embyClientName, credentials.DeviceID, embyClientVersion,
	)
	if credentials.AccessToken != "" {
		authorization += fmt.Sprintf(`, Token="%s"`, credentials.AccessToken)
		request.Header.Set("X-Emby-Token", credentials.AccessToken)
	}
	request.Header.Set("X-Emby-Authorization", authorization)
	return request, nil
}

func (m *MediaSourceManager) logoutEmby(ctx context.Context, source MediaSource, secret []byte) {
	credentials, err := decodeEmbyCredentials(secret)
	if err != nil {
		return
	}
	request, err := m.newEmbyRequest(
		ctx, source.BaseURL, credentials, http.MethodPost, []string{"Sessions", "Logout"}, nil, nil,
	)
	if err != nil {
		return
	}
	response, err := m.client.Do(request)
	if err == nil {
		response.Body.Close()
	}
}

func decodeEmbyCredentials(secret []byte) (embyCredentials, error) {
	var credentials embyCredentials
	if json.Unmarshal(secret, &credentials) != nil ||
		!validEmbyIdentifier(credentials.UserID) ||
		!validEmbyToken(credentials.AccessToken) ||
		!validEmbyIdentifier(credentials.DeviceID) {
		return embyCredentials{}, errors.New("invalid stored Emby credentials")
	}
	return credentials, nil
}

func decodeEmbyJSON(reader io.Reader, target any) error {
	payload, err := io.ReadAll(io.LimitReader(reader, embyResponseLimit+1))
	if err != nil || len(payload) > embyResponseLimit || json.Unmarshal(payload, target) != nil {
		return errors.New("invalid Emby response")
	}
	return nil
}

func embyEndpoint(baseURL string, segments ...string) (*url.URL, error) {
	target, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	parts := append([]string{target.Path}, segments...)
	target.Path = "/" + strings.TrimPrefix(path.Join(parts...), "/")
	target.RawPath = ""
	target.RawQuery = ""
	target.Fragment = ""
	return target, nil
}

func embyItemIDFromPath(value string, allowRoot bool) (string, error) {
	if len(value) > 4096 {
		return "", errors.New("Emby item path is too long")
	}
	for _, segment := range strings.Split(strings.ReplaceAll(value, "\\", "/"), "/") {
		if segment == ".." {
			return "", errors.New("parent path traversal is not allowed")
		}
	}
	clean := cleanMediaPath(value)
	if clean == "/" {
		if allowRoot {
			return "", nil
		}
		return "", errors.New("an Emby media item is required")
	}
	segments := strings.Split(strings.Trim(clean, "/"), "/")
	if len(segments) > 64 {
		return "", errors.New("Emby item path is too deep")
	}
	for _, segment := range segments {
		if !validEmbyIdentifier(segment) {
			return "", errors.New("invalid Emby item path")
		}
	}
	return segments[len(segments)-1], nil
}

func appendEmbyItemPath(parent, itemID string) string {
	parent = strings.TrimSuffix(cleanMediaPath(parent), "/")
	if parent == "" || parent == "/" {
		return "/" + itemID
	}
	return parent + "/" + itemID
}

func embySubtitlePath(mediaPath string, index int, format string) string {
	return strings.TrimSuffix(cleanMediaPath(mediaPath), "/") +
		"/@subtitle/" + strconv.Itoa(index) + "/" + format
}

func parseEmbySubtitlePath(mediaPath, subtitlePath string) (int, string, error) {
	prefix := strings.TrimSuffix(cleanMediaPath(mediaPath), "/") + "/@subtitle/"
	clean := cleanMediaPath(subtitlePath)
	if !strings.HasPrefix(clean, prefix) {
		return 0, "", ErrForbidden
	}
	parts := strings.Split(strings.TrimPrefix(clean, prefix), "/")
	if len(parts) != 2 || parts[1] != "vtt" {
		return 0, "", ErrForbidden
	}
	index, err := strconv.Atoi(parts[0])
	if err != nil || index < 0 {
		return 0, "", ErrForbidden
	}
	return index, parts[1], nil
}

func embyItemIsVideo(item embyItem) bool {
	if strings.EqualFold(item.MediaType, "Video") {
		return true
	}
	switch strings.ToLower(item.Type) {
	case "movie", "episode", "video", "musicvideo", "trailer":
		return true
	default:
		return false
	}
}

func normalizeEmbyContainer(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.Split(value, ",")[0]))
	if !embyContainerPattern.MatchString(value) {
		return ""
	}
	return value
}

func embyContentType(container string) string {
	switch normalizeEmbyContainer(container) {
	case "mp4", "m4v":
		return "video/mp4"
	case "mkv":
		return "video/x-matroska"
	case "webm":
		return "video/webm"
	case "ts", "m2ts":
		return "video/mp2t"
	case "avi":
		return "video/x-msvideo"
	default:
		return "application/octet-stream"
	}
}

func validEmbyIdentifier(value string) bool {
	return value != "." && value != ".." && embyIdentifierPattern.MatchString(value)
}

func validEmbyToken(value string) bool {
	return value != "" && len(value) <= 4096 &&
		!strings.ContainsAny(value, "\"\\") && !strings.ContainsFunc(value, unicode.IsControl)
}
