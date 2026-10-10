package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

type NASSourceInput struct {
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Token     string `json:"token"`
	OTP       string `json:"otp"`
	WebDAVURL string `json:"webdav_url"`
}

type nasCredentials struct {
	Username   string            `json:"username,omitempty"`
	Password   string            `json:"password,omitempty"`
	Token      string            `json:"token,omitempty"`
	SID        string            `json:"sid,omitempty"`
	Secret     string            `json:"secret,omitempty"`
	LongToken  string            `json:"long_token,omitempty"`
	WebDAVURL  string            `json:"webdav_url,omitempty"`
	WebDAVRoot string            `json:"webdav_root,omitempty"`
	APIs       map[string]nasAPI `json:"apis,omitempty"`
}

type nasAPI struct {
	Path       string `json:"path"`
	MaxVersion int    `json:"maxVersion"`
}

func isNASProvider(kind string) bool {
	switch kind {
	case "synology", "qnap", "fnos", "nextcloud", "seafile", "truenas":
		return true
	}
	return false
}

func (m *MediaSourceManager) SaveNAS(ctx context.Context, user User, sourceID string, in NASSourceInput) (MediaSource, error) {
	if !isNASProvider(in.Provider) {
		return MediaSource{}, errors.New("不支持的 NAS 类型")
	}
	in.Name = strings.TrimSpace(in.Name)
	if len([]rune(in.Name)) < 1 || len([]rune(in.Name)) > 64 || len(in.Username) > 256 || len(in.Password) > 2048 || len(in.Token) > 4096 || len(in.OTP) > 12 {
		return MediaSource{}, errors.New("NAS 连接信息格式无效")
	}
	base, err := validateSourceBaseURL(ctx, in.BaseURL, m.allowPrivate)
	if err != nil {
		return MediaSource{}, err
	}
	var source MediaSource
	if sourceID != "" {
		source, err = m.repository.GetMediaSource(ctx, user.ID, sourceID)
		if err != nil {
			return source, err
		}
		if source.Type != in.Provider || source.BaseURL != base.String() {
			return MediaSource{}, errors.New("重新登录时请保留原 NAS 类型和地址")
		}
	}
	c := nasCredentials{Username: in.Username}
	probe := MediaSource{Type: in.Provider, BaseURL: base.String()}
	switch in.Provider {
	case "synology":
		var info map[string]any
		if err = m.nasJSON(ctx, probe, http.MethodPost, "webapi/query.cgi", nil, url.Values{"api": {"SYNO.API.Info"}, "version": {"1"}, "method": {"query"}, "query": {"SYNO.API.Auth,SYNO.FileStation.List,SYNO.FileStation.Download"}}, "", &info); err != nil {
			return source, err
		}
		if !boolValue(info["success"]) {
			return source, errors.New("无法读取群晖 API 配置")
		}
		encoded, _ := json.Marshal(info["data"])
		if json.Unmarshal(encoded, &c.APIs) != nil {
			return source, errors.New("群晖 API 配置无效")
		}
		for _, name := range []string{"SYNO.API.Auth", "SYNO.FileStation.List", "SYNO.FileStation.Download"} {
			api, ok := c.APIs[name]
			if !ok || !validNASAPIPath(api.Path) {
				return source, errors.New("群晖未提供所需的文件服务 API")
			}
		}
		form := synologyForm(c, "SYNO.API.Auth", "login")
		form.Set("account", in.Username)
		form.Set("passwd", in.Password)
		form.Set("session", "FileStation")
		form.Set("format", "sid")
		if in.OTP != "" {
			form.Set("otp_code", in.OTP)
		}
		data, err := m.synologyJSON(ctx, probe, c, "SYNO.API.Auth", form)
		if err != nil {
			return source, err
		}
		c.SID = textValue(data["sid"])
		if c.SID == "" {
			return source, errors.New("群晖未返回登录会话")
		}
	case "qnap":
		var response map[string]any
		if err = m.nasJSON(ctx, probe, http.MethodGet, "cgi-bin/filemanager/wfm2Login.cgi", url.Values{"user": {in.Username}, "pwd": {base64.StdEncoding.EncodeToString([]byte(in.Password))}}, nil, "", &response); err != nil {
			return source, err
		}
		if numberValue(response["status"]) != 1 || textValue(response["sid"]) == "" {
			return source, errors.New("QNAP 登录失败，请检查账号或使用 File Station 支持的登录方式")
		}
		c.SID = textValue(response["sid"])
	case "nextcloud":
		c.Password = in.Password
		request, err := m.nasRequest(ctx, probe, http.MethodGet, "ocs/v2.php/cloud/user", url.Values{"format": {"json"}}, nil)
		if err != nil {
			return source, err
		}
		request.SetBasicAuth(in.Username, in.Password)
		request.Header.Set("OCS-APIRequest", "true")
		var result map[string]any
		if err = m.readNASJSON(request, &result); err != nil {
			return source, err
		}
		ocs := objectValue(result["ocs"])
		meta := objectValue(ocs["meta"])
		if numberValue(meta["statuscode"]) != 100 && numberValue(meta["statuscode"]) != 200 {
			return source, errors.New("Nextcloud 登录失败，请使用应用密码")
		}
		account := textValue(objectValue(ocs["data"])["id"])
		if account == "" {
			return source, errors.New("Nextcloud 未返回账号标识")
		}
		dav, _ := url.Parse(probe.BaseURL)
		dav.Path = path.Join(dav.Path, "remote.php/dav/files", account) + "/"
		c.WebDAVURL = dav.String()
	case "seafile":
		var response map[string]any
		if err = m.nasJSON(ctx, probe, http.MethodPost, "api2/auth-token/", nil, url.Values{"username": {in.Username}, "password": {in.Password}}, "", &response); err != nil {
			return source, err
		}
		c.Token = textValue(response["token"])
		if c.Token == "" {
			return source, errors.New("Seafile 登录失败")
		}
	case "truenas":
		c.Token = strings.TrimSpace(in.Token)
		if c.Token == "" {
			return source, errors.New("请填写 TrueNAS API Key")
		}
		var response map[string]any
		if err = m.nasJSON(ctx, probe, http.MethodGet, "api/v2.0/system/info/", nil, nil, "Bearer "+c.Token, &response); err != nil {
			return source, err
		}
	case "fnos":
		c, err = m.loginFnos(ctx, probe, in)
		if err != nil {
			return source, err
		}
	}
	secret, _ := json.Marshal(c)
	// Verify the read capability before publishing the binding.
	if _, err = m.browseNAS(ctx, probe, secret, "/"); err != nil {
		return source, err
	}
	if sourceID == "" {
		now := time.Now().UnixMilli()
		source = MediaSource{ID: mustRandomString(12), UserID: user.ID, Type: in.Provider, Name: in.Name, BaseURL: base.String(), CreatedAt: now, UpdatedAt: now}
	}
	encrypted, err := m.vault.Encrypt(secret, sourceAAD(user.ID, source.ID))
	if err != nil {
		return source, err
	}
	if sourceID == "" {
		source.CredentialsCiphertext = encrypted
		err = m.repository.CreateMediaSource(ctx, source)
	} else {
		var replaced bool
		replaced, err = m.repository.ReplaceMediaSourceCredentials(ctx, user.ID, source.ID, source.CredentialsCiphertext, encrypted)
		if err == nil && !replaced {
			err = ErrRoomConflict
		}
		source.CredentialsCiphertext = encrypted
	}
	return source, err
}

func validNASAPIPath(value string) bool {
	return value != "" && !strings.ContainsAny(value, "/\\?#") && strings.HasSuffix(value, ".cgi")
}
func textValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
func numberValue(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}
func boolValue(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return numberValue(v) == 1
}
func objectValue(v any) map[string]any { m, _ := v.(map[string]any); return m }
func arrayValue(v any) []any           { a, _ := v.([]any); return a }

func (m *MediaSourceManager) nasRequest(ctx context.Context, source MediaSource, method, route string, query url.Values, body any) (*http.Request, error) {
	base, err := url.Parse(source.BaseURL)
	if err != nil {
		return nil, errors.New("NAS 地址无效")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(route, "/")
	base.RawPath = ""
	base.RawQuery = query.Encode()
	var reader io.Reader
	contentType := ""
	if form, ok := body.(url.Values); ok {
		reader = strings.NewReader(form.Encode())
		contentType = "application/x-www-form-urlencoded"
	} else if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
		contentType = "application/json"
	}
	request, err := http.NewRequestWithContext(ctx, method, base.String(), reader)
	if err != nil {
		return nil, errors.New("NAS 请求无效")
	}
	request.Header.Set("Accept", "application/json")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	return request, nil
}

func (m *MediaSourceManager) readNASJSON(request *http.Request, out any) error {
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	request = request.WithContext(ctx)
	response, err := m.client.Do(request)
	if err != nil {
		return errors.New("无法连接 NAS，请检查地址和网络")
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return errors.New("NAS 登录状态已失效，请重新连接")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("NAS 请求失败（HTTP %d）", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20+1))
	if err != nil || len(data) > 8<<20 || json.Unmarshal(data, out) != nil {
		return errors.New("NAS 返回了无效或过大的响应")
	}
	return nil
}

func (m *MediaSourceManager) nasJSON(ctx context.Context, source MediaSource, method, route string, query url.Values, body any, authorization string, out any) error {
	request, err := m.nasRequest(ctx, source, method, route, query, body)
	if err != nil {
		return err
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	return m.readNASJSON(request, out)
}

func synologyForm(c nasCredentials, api, method string) url.Values {
	version := min(c.APIs[api].MaxVersion, 2)
	if api == "SYNO.API.Auth" {
		version = min(c.APIs[api].MaxVersion, 7)
	}
	if version < 1 {
		version = 1
	}
	f := url.Values{"api": {api}, "version": {strconv.Itoa(version)}, "method": {method}}
	if c.SID != "" {
		f.Set("_sid", c.SID)
	}
	return f
}

func (m *MediaSourceManager) synologyJSON(ctx context.Context, source MediaSource, c nasCredentials, api string, form url.Values) (map[string]any, error) {
	definition, ok := c.APIs[api]
	if !ok || !validNASAPIPath(definition.Path) {
		return nil, errors.New("群晖 API 配置无效")
	}
	var response map[string]any
	if err := m.nasJSON(ctx, source, http.MethodPost, "webapi/"+definition.Path, nil, form, "", &response); err != nil {
		return nil, err
	}
	if !boolValue(response["success"]) {
		return nil, fmt.Errorf("群晖操作失败（%d），请检查登录状态与文件权限", numberValue(objectValue(response["error"])["code"]))
	}
	return objectValue(response["data"]), nil
}

func safeNASPath(value string) (string, error) {
	if len(value) > 4096 || strings.ContainsAny(value, "\x00\\") {
		return "", errors.New("文件路径无效")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return "", errors.New("不允许访问上级路径")
		}
	}
	return cleanMediaPath(value), nil
}

func (m *MediaSourceManager) browseNAS(ctx context.Context, source MediaSource, secret []byte, requested string) ([]MediaFile, error) {
	var c nasCredentials
	if json.Unmarshal(secret, &c) != nil {
		return nil, errors.New("NAS 登录信息无效")
	}
	clean, err := safeNASPath(requested)
	if err != nil {
		return nil, err
	}
	files := []MediaFile{}
	switch source.Type {
	case "nextcloud":
		dav := source
		dav.BaseURL = c.WebDAVURL
		return m.browseWebDAV(ctx, dav, webDAVCredentials{Username: c.Username, Password: c.Password}, clean)
	case "fnos":
		return m.browseFnos(ctx, source, c, clean)
	case "synology":
		for offset := 0; offset < 5000; offset += 100 {
			method := "list"
			if clean == "/" {
				method = "list_share"
			}
			form := synologyForm(c, "SYNO.FileStation.List", method)
			form.Set("offset", strconv.Itoa(offset))
			form.Set("limit", "100")
			form.Set("additional", `["size","time"]`)
			form.Set("sort_by", "name")
			form.Set("sort_direction", "asc")
			if clean != "/" {
				form.Set("folder_path", clean)
			}
			data, err := m.synologyJSON(ctx, source, c, "SYNO.FileStation.List", form)
			if err != nil {
				return nil, err
			}
			rows := arrayValue(data["files"])
			if clean == "/" {
				rows = arrayValue(data["shares"])
			}
			for _, raw := range rows {
				v := objectValue(raw)
				file := MediaFile{Name: textValue(v["name"]), Path: textValue(v["path"]), IsDirectory: boolValue(v["isdir"]), Size: numberValue(objectValue(v["additional"])["size"])}
				files = append(files, file)
			}
			if len(rows) < 100 || int64(offset+len(rows)) >= numberValue(data["total"]) {
				break
			}
			if offset == 4900 {
				return nil, errors.New("目录文件过多，请整理为子目录")
			}
		}
	case "qnap":
		if clean == "/" {
			var rows []map[string]any
			if err = m.nasJSON(ctx, source, http.MethodGet, "cgi-bin/filemanager/utilRequest.cgi", url.Values{"func": {"get_tree"}, "node": {"share_root"}, "sid": {c.SID}}, nil, "", &rows); err != nil {
				return nil, err
			}
			for _, v := range rows {
				p := textValue(v["id"])
				if p == "" {
					p = textValue(v["text"])
				}
				files = append(files, MediaFile{Name: textValue(v["text"]), Path: cleanMediaPath(p), IsDirectory: true})
			}
		} else {
			for offset := 0; offset < 5000; offset += 100 {
				var response map[string]any
				if err = m.nasJSON(ctx, source, http.MethodGet, "cgi-bin/filemanager/utilRequest.cgi", url.Values{"func": {"get_list"}, "sid": {c.SID}, "path": {clean}, "start": {strconv.Itoa(offset)}, "limit": {"100"}, "list_mode": {"all"}, "hidden_file": {"0"}, "sort": {"filename"}, "dir": {"ASC"}}, nil, "", &response); err != nil {
					return nil, err
				}
				if status, ok := response["status"]; ok && numberValue(status) != 1 {
					return nil, errors.New("QNAP 目录读取失败，请重新登录或检查权限")
				}
				rows := arrayValue(response["datas"])
				for _, raw := range rows {
					v := objectValue(raw)
					name := textValue(v["filename"])
					files = append(files, MediaFile{Name: name, Path: path.Join(clean, name), IsDirectory: numberValue(v["isfolder"]) == 1, Size: numberValue(v["filesize"])})
				}
				if len(rows) < 100 {
					break
				}
				if offset == 4900 {
					return nil, errors.New("目录文件过多，请整理为子目录")
				}
			}
		}
	case "seafile":
		if clean == "/" {
			var repos []map[string]any
			if err = m.nasJSON(ctx, source, http.MethodGet, "api2/repos/", nil, nil, "Token "+c.Token, &repos); err != nil {
				return nil, err
			}
			for _, repo := range repos {
				if boolValue(repo["encrypted"]) {
					continue
				}
				id := textValue(repo["id"])
				if quarkFileIDPattern.MatchString(id) {
					files = append(files, MediaFile{Name: textValue(repo["name"]), Path: "/" + id, IsDirectory: true})
				}
			}
		} else {
			parts := strings.SplitN(strings.TrimPrefix(clean, "/"), "/", 2)
			if !quarkFileIDPattern.MatchString(parts[0]) {
				return nil, errors.New("文件库标识无效")
			}
			directory := "/"
			if len(parts) > 1 {
				directory += "" + parts[1]
			}
			var response any
			if err = m.nasJSON(ctx, source, http.MethodGet, "api/v2.1/repos/"+parts[0]+"/dir/", url.Values{"p": {directory}}, nil, "Token "+c.Token, &response); err != nil {
				return nil, err
			}
			rows := arrayValue(response)
			if objectValue(response) != nil {
				rows = arrayValue(objectValue(response)["dirent_list"])
			}
			for _, raw := range rows {
				v := objectValue(raw)
				name := textValue(v["name"])
				files = append(files, MediaFile{Name: name, Path: path.Join(clean, name), IsDirectory: textValue(v["type"]) == "dir", Size: numberValue(v["size"])})
			}
		}
	case "truenas":
		if clean == "/" {
			clean = "/mnt"
		}
		if clean != "/mnt" && !strings.HasPrefix(clean, "/mnt/") {
			return nil, errors.New("TrueNAS 仅允许浏览 /mnt 下的存储")
		}
		var rows []map[string]any
		if err = m.nasJSON(ctx, source, http.MethodGet, "api/v2.0/filesystem/listdir/", url.Values{"path": {clean}}, nil, "Bearer "+c.Token, &rows); err != nil {
			return nil, err
		}
		for _, v := range rows {
			name := textValue(v["name"])
			files = append(files, MediaFile{Name: name, Path: path.Join(clean, name), IsDirectory: textValue(v["type"]) == "DIRECTORY", Size: numberValue(v["size"])})
		}
	default:
		return nil, errors.New("不支持的 NAS 类型")
	}
	return cleanNASFiles(files), nil
}

func cleanNASFiles(files []MediaFile) []MediaFile {
	result := []MediaFile{}
	for _, file := range files {
		if file.Name == "" || strings.ContainsAny(file.Name, "/\\\x00") || file.Name == "." || file.Name == ".." {
			continue
		}
		clean, err := safeNASPath(file.Path)
		if err != nil {
			continue
		}
		file.Path = clean
		file.Name = sanitizeAuditText(file.Name, 256)
		file.Size = max(file.Size, 0)
		if file.IsDirectory || quarkIsVideo(quarkFile{Name: file.Name}) || isSubtitlePath(file.Name) {
			result = append(result, file)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].IsDirectory != result[j].IsDirectory {
			return result[i].IsDirectory
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func (m *MediaSourceManager) prepareNASTicket(ctx context.Context, source MediaSource, secret []byte, requested string) (MediaTicket, error) {
	clean, err := safeNASPath(requested)
	if err != nil {
		return MediaTicket{}, err
	}
	if clean == "/" {
		return MediaTicket{}, errors.New("请选择文件")
	}
	files, err := m.browseNAS(ctx, source, secret, path.Dir(clean))
	if err != nil {
		return MediaTicket{}, err
	}
	for _, file := range files {
		if !file.IsDirectory && file.Path == clean {
			return MediaTicket{UserID: source.UserID, SourceID: source.ID, Path: clean, Kind: "nas-file"}, nil
		}
	}
	return MediaTicket{}, errors.New("所选文件不存在或没有访问权限")
}

func (m *MediaSourceManager) openNAS(ctx context.Context, source MediaSource, secret []byte, ticket MediaTicket, method, rangeHeader, ifRange string) (*http.Response, error) {
	if ticket.Kind != "nas-file" || (method != http.MethodGet && method != http.MethodHead) {
		return nil, errors.New("invalid NAS ticket")
	}
	clean, err := safeNASPath(ticket.Path)
	if err != nil {
		return nil, err
	}
	var c nasCredentials
	if json.Unmarshal(secret, &c) != nil {
		return nil, errors.New("NAS 登录信息无效")
	}
	var request *http.Request
	switch source.Type {
	case "synology":
		form := synologyForm(c, "SYNO.FileStation.Download", "download")
		form.Set("path", clean)
		form.Set("mode", "download")
		api := c.APIs["SYNO.FileStation.Download"]
		if !validNASAPIPath(api.Path) {
			return nil, errors.New("群晖 API 配置无效")
		}
		request, err = m.nasRequest(ctx, source, method, "webapi/"+api.Path, form, nil)
	case "qnap":
		request, err = m.nasRequest(ctx, source, method, "cgi-bin/filemanager/utilRequest.cgi", url.Values{"func": {"download"}, "sid": {c.SID}, "isfolder": {"0"}, "compress": {"0"}, "source_path": {path.Dir(clean)}, "source_file": {path.Base(clean)}, "source_total": {"1"}}, nil)
	case "nextcloud", "fnos":
		dav := source
		dav.BaseURL = c.WebDAVURL
		if source.Type == "fnos" {
			clean = fnosDAVPath(clean, c.WebDAVRoot)
		}
		target, resolveErr := resolveSourcePath(dav.BaseURL, clean, false)
		if resolveErr != nil {
			return nil, resolveErr
		}
		request, err = http.NewRequestWithContext(ctx, method, target.String(), nil)
		if err == nil {
			request.SetBasicAuth(c.Username, c.Password)
		}
	case "seafile":
		parts := strings.SplitN(strings.TrimPrefix(clean, "/"), "/", 2)
		if len(parts) != 2 || !quarkFileIDPattern.MatchString(parts[0]) {
			return nil, errors.New("文件库路径无效")
		}
		var link string
		if err = m.nasJSON(ctx, source, http.MethodGet, "api2/repos/"+parts[0]+"/file/", url.Values{"p": {"/" + parts[1]}, "reuse": {"1"}}, nil, "Token "+c.Token, &link); err != nil {
			return nil, err
		}
		request, err = m.signedNASRequest(ctx, source, method, link)
	case "truenas":
		if !strings.HasPrefix(clean, "/mnt/") {
			return nil, errors.New("TrueNAS 文件必须位于 /mnt 下")
		}
		var stat map[string]any
		if err = m.nasJSON(ctx, source, http.MethodPost, "api/v2.0/filesystem/stat/", nil, map[string]string{"path": clean}, "Bearer "+c.Token, &stat); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(textValue(stat["realpath"]), "/mnt/") {
			return nil, errors.New("文件超出 TrueNAS 存储范围")
		}
		var response []any
		if err = m.nasJSON(ctx, source, http.MethodPost, "api/v2.0/core/download/", nil, map[string]any{"method": "filesystem.get", "args": []string{clean}, "filename": path.Base(clean), "buffered": false}, "Bearer "+c.Token, &response); err != nil {
			return nil, err
		}
		if len(response) != 2 {
			return nil, errors.New("TrueNAS 下载响应无效")
		}
		request, err = m.signedNASRequest(ctx, source, method, textValue(response[1]))
	default:
		return nil, errors.New("不支持的 NAS 类型")
	}
	if err != nil {
		return nil, errors.New("无法准备 NAS 播放请求")
	}
	if rangeHeader != "" {
		request.Header.Set("Range", rangeHeader)
	}
	if ifRange != "" {
		request.Header.Set("If-Range", ifRange)
	}
	request.Header.Del("Accept")
	response, err := m.client.Do(request)
	if err != nil {
		return nil, errors.New("NAS 视频连接失败，请检查网络或重新登录")
	}
	return response, nil
}

func (m *MediaSourceManager) signedNASRequest(ctx context.Context, source MediaSource, method, raw string) (*http.Request, error) {
	base, _ := url.Parse(source.BaseURL)
	target, err := base.Parse(raw)
	if err != nil || target.User != nil || target.Hostname() != base.Hostname() || (target.Scheme != "https" && !(m.allowPrivate && target.Scheme == "http")) {
		return nil, errors.New("NAS 下载地址不受信任")
	}
	return http.NewRequestWithContext(ctx, method, target.String(), nil)
}
