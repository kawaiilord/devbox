package app

import (
	"errors"
	"net/http"
	"sort"
	"time"
)

func (s *Server) providerCatalog(w http.ResponseWriter, r *http.Request) {
	items := []map[string]any{}
	for _, id := range []string{"synology", "fnos", "qnap", "nextcloud", "seafile", "truenas"} {
		names := map[string]string{"synology": "群晖 Synology", "fnos": "飞牛 fnOS", "qnap": "QNAP", "nextcloud": "Nextcloud", "seafile": "Seafile", "truenas": "TrueNAS"}
		items = append(items, map[string]any{"id": id, "name": names[id], "kind": "nas", "available": s.sources != nil})
	}
	keys := []string{}
	for key := range platformDefinitions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		d := platformDefinitions[key]
		items = append(items, map[string]any{"id": key, "name": d.Name, "kind": "platform", "available": s.sources != nil && s.sources.platform.available()})
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"providers": items}})
}

func (s *Server) providerOwner(w http.ResponseWriter, r *http.Request) (User, bool) {
	if s.sources == nil {
		writeError(w, 503, errors.New("媒体源服务未配置"))
		return User{}, false
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return User{}, false
	}
	if s.options.RequireVerifiedEmail && !user.EmailVerified {
		writeError(w, 403, errors.New("email verification required"))
		return User{}, false
	}
	if !s.enforceRateLimit(w, r, "source-create", user.ID, 20, time.Hour) {
		return User{}, false
	}
	return user, true
}

func (s *Server) saveNASSource(w http.ResponseWriter, r *http.Request) {
	user, ok := s.providerOwner(w, r)
	if !ok {
		return
	}
	var in NASSourceInput
	if decodeJSON(r, &in) != nil {
		writeError(w, 400, errors.New("NAS 连接信息格式无效"))
		return
	}
	source, err := s.sources.SaveNAS(r.Context(), user, r.PathValue("id"), in)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, apiResponse{Code: 0, Data: source})
}

func (s *Server) savePlatformSource(w http.ResponseWriter, r *http.Request) {
	user, ok := s.providerOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		Provider string `json:"provider"`
		Name     string `json:"name"`
		Cookie   string `json:"cookie"`
	}
	if decodeJSON(r, &in) != nil {
		writeError(w, 400, errors.New("平台连接信息格式无效"))
		return
	}
	source, err := s.sources.SavePlatform(r.Context(), user, r.PathValue("id"), in.Provider, in.Name, in.Cookie)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, apiResponse{Code: 0, Data: source})
}

func (s *Server) resolvePlatformSource(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil {
		writeError(w, 503, errors.New("媒体源服务未配置"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	if !s.enforceRateLimit(w, r, "platform-resolve", user.ID, 30, time.Minute) {
		return
	}
	var in struct {
		URL    string `json:"url"`
		Offset int    `json:"offset"`
	}
	if decodeJSON(r, &in) != nil {
		writeError(w, 400, errors.New("视频链接格式无效"))
		return
	}
	files, err := s.sources.ResolvePlatform(r.Context(), user.ID, r.PathValue("id"), in.URL, in.Offset)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"files": files, "limit": 50}})
}
