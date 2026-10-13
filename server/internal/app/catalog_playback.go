package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type catalogBinding struct {
	CatalogID string `json:"catalog_id"`
}
type CatalogRef struct {
	SourceID string `json:"source_id"`
	FilmID   string `json:"film_id"`
	GroupID  string `json:"group_id"`
}
type CatalogSelection struct {
	Refs        []CatalogRef `json:"refs"`
	EpisodeID   string       `json:"episode_id"`
	AllEpisodes bool         `json:"all_episodes"`
}

func (c *CatalogService) bind(ctx context.Context, user User, src CatalogSource) (MediaSource, error) {
	id := "cat_" + catalogHash(user.ID+":"+src.ID)
	if v, e := c.repo.GetMediaSource(ctx, user.ID, id); e == nil {
		if v.Disabled {
			return v, errors.New("此片源绑定已停用")
		}
		return v, nil
	}
	if c.vault == nil {
		return MediaSource{}, errors.New("媒体订阅密钥库未配置")
	}
	raw, _ := json.Marshal(catalogBinding{CatalogID: src.ID})
	secret, e := c.vault.Encrypt(raw, sourceAAD(user.ID, id))
	if e != nil {
		return MediaSource{}, e
	}
	now := c.now().UnixMilli()
	source := MediaSource{ID: id, UserID: user.ID, Type: "catalog", Name: src.Name, CredentialsCiphertext: secret, CreatedAt: now, UpdatedAt: now}
	if e = c.repo.CreateMediaSource(ctx, source); e != nil {
		if v, e2 := c.repo.GetMediaSource(ctx, user.ID, id); e2 == nil && !v.Disabled {
			return v, nil
		}
		return MediaSource{}, e
	}
	return source, nil
}
func (c *CatalogService) boundSource(ctx context.Context, secret []byte) (CatalogSource, error) {
	var b catalogBinding
	if json.Unmarshal(secret, &b) != nil || b.CatalogID == "" {
		return CatalogSource{}, ErrForbidden
	}
	return c.source(ctx, b.CatalogID)
}
func catalogPath(id, group, episode string) string { return "/" + id + "/" + group + "/" + episode }
func (c *CatalogService) episode(ctx context.Context, src CatalogSource, path string) (CatalogEpisode, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 3 {
		return CatalogEpisode{}, ErrNotFound
	}
	f, e := c.detail(ctx, src, parts[0])
	if e != nil {
		return CatalogEpisode{}, e
	}
	for _, group := range f.Groups {
		if group.ID == parts[1] {
			for _, ep := range group.Episodes {
				if ep.ID == parts[2] {
					return ep, nil
				}
			}
		}
	}
	return CatalogEpisode{}, ErrNotFound
}
func (c *CatalogService) mediaRequest(ctx context.Context, target, method, rangeHeader, ifRange string) (*http.Response, error) {
	u, e := catalogURL(target)
	if e != nil {
		return nil, e
	}
	r, e := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if e != nil {
		return nil, ErrForbidden
	}
	r.Header.Set("User-Agent", "SameFrame/1.0")
	if rangeHeader != "" {
		r.Header.Set("Range", rangeHeader)
	}
	if ifRange != "" {
		r.Header.Set("If-Range", ifRange)
	}
	client := *c.client
	client.Timeout = 0
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("媒体跳转过多")
		}
		if _, e := catalogURL(next.URL.String()); e != nil {
			return e
		}
		if len(via) > 0 && via[0].URL.Scheme == "https" && next.URL.Scheme != "https" {
			return errors.New("媒体跳转降级被拒绝")
		}
		return nil
	}
	return client.Do(r)
}
func (c *CatalogService) probeMedia(ctx context.Context, raw string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	r, e := c.mediaRequest(ctx, raw, "GET", "bytes=0-1023", "")
	if e != nil {
		return "", errors.New("播放连接失败")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 && r.StatusCode != 206 {
		return "", errors.New("播放地址不可访问")
	}
	buf, e := io.ReadAll(io.LimitReader(r.Body, 1024))
	if e != nil || len(buf) == 0 {
		return "", errors.New("播放地址没有返回媒体内容")
	}
	s := strings.TrimSpace(strings.TrimPrefix(string(buf), "\ufeff"))
	if strings.HasPrefix(s, "#EXTM3U") {
		return "platform-hls", nil
	}
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "text/html") || strings.HasPrefix(strings.ToLower(s), "<!doctype") || strings.HasPrefix(s, "<html") || strings.Contains(ct, "application/json") {
		return "", errors.New("该线路是网页解析地址，请选择可直连的媒体线路")
	}
	return "platform-file", nil
}
func (m *MediaSourceManager) prepareCatalog(ctx context.Context, source MediaSource, secret []byte, path string) (MediaTicket, error) {
	src, e := m.catalog.boundSource(ctx, secret)
	if e != nil {
		return MediaTicket{}, e
	}
	episode, e := m.catalog.episode(ctx, src, path)
	if e != nil {
		return MediaTicket{}, e
	}
	kind := "platform-file"
	// Probe once per short-lived cache window to identify extensionless HLS URLs.
	key := "media:" + catalogHash(episode.URL)
	m.catalog.mu.Lock()
	cached, ok := m.catalog.cache[key]
	m.catalog.mu.Unlock()
	if ok && cached.Expires.After(m.catalog.now()) {
		kind = string(cached.Data)
	} else {
		kind, e = m.catalog.probeMedia(ctx, episode.URL)
		if e != nil {
			return MediaTicket{}, e
		}
		m.catalog.mu.Lock()
		if len(m.catalog.cache) >= 128 {
			for k := range m.catalog.cache {
				delete(m.catalog.cache, k)
				break
			}
		}
		m.catalog.cache[key] = catalogCacheEntry{Data: []byte(kind), Expires: m.catalog.now().Add(time.Minute)}
		m.catalog.mu.Unlock()
	}
	return MediaTicket{UserID: source.UserID, SourceID: source.ID, Path: path, Kind: kind, StreamURL: episode.URL, IsLive: episode.Live}, nil
}

var catalogEpisodeNumber = regexp.MustCompile(`(?i)^(?:第|ep(?:isode)?\s*)?0*([0-9]+)(?:集|话|話|期)?$`)

func catalogEpisodeKey(ep CatalogEpisode, kind string) string {
	if m := catalogEpisodeNumber.FindStringSubmatch(strings.TrimSpace(ep.Name)); m != nil {
		n, _ := strconv.Atoi(m[1])
		return "episode:" + strconv.Itoa(n)
	}
	if kind == "movie" && (ep.Name == "正片" || ep.Name == "HD" || ep.Name == "HD中字") {
		return "feature"
	}
	return catalogNormalize(ep.Name)
}
func (c *CatalogService) playlist(ctx context.Context, user User, in CatalogSelection) ([]PlaylistEntry, int, error) {
	if len(in.Refs) < 1 || len(in.Refs) > 8 {
		return nil, 0, errors.New("请选择 1–8 条同片线路")
	}
	type selected struct {
		film    CatalogFilm
		group   CatalogGroup
		binding MediaSource
	}
	chosen := []selected{}
	key := ""
	for index, ref := range in.Refs {
		src, e := c.source(ctx, ref.SourceID)
		if e != nil {
			return nil, 0, e
		}
		film, e := c.detail(ctx, src, ref.FilmID)
		if e != nil {
			if index > 0 {
				continue
			}
			return nil, 0, e
		}
		if index == 0 {
			key = film.GroupKey
		} else if film.GroupKey != key {
			return nil, 0, errors.New("不同年份或版本的影片不能合并为同一集片源")
		}
		var g CatalogGroup
		for _, group := range film.Groups {
			if ref.GroupID == group.ID || (ref.GroupID == "" && g.ID == "") {
				g = group
			}
		}
		if g.ID == "" {
			if index > 0 {
				continue
			}
			return nil, 0, errors.New("没有可直连的播放线路")
		}
		bound, e := c.bind(ctx, user, src)
		if e != nil {
			return nil, 0, e
		}
		chosen = append(chosen, selected{film, g, bound})
	}
	main := chosen[0]
	items := []PlaylistEntry{}
	active := -1
	for _, ep := range main.group.Episodes {
		if !in.AllEpisodes && ep.ID != in.EpisodeID {
			continue
		}
		item := PlaylistEntry{ID: mustRandomString(10), Title: catalogText(main.film.Title+" · "+ep.Name, 256), Episode: len(items), Sources: []PlaylistSource{}}
		for i, source := range chosen {
			matches := []CatalogEpisode{}
			for _, other := range source.group.Episodes {
				if (i == 0 && other.ID == ep.ID) || (i > 0 && catalogEpisodeKey(other, main.film.Kind) == catalogEpisodeKey(ep, main.film.Kind)) {
					matches = append(matches, other)
				}
			}
			if len(matches) != 1 {
				continue
			}
			other := matches[0]
			ps := PlaylistSource{ID: mustRandomString(10), OwnerID: user.ID, Label: catalogText(source.film.SourceName+" · "+source.group.Name, 80), MediaSourceID: source.binding.ID, MediaPath: catalogPath(source.film.ID, source.group.ID, other.ID), IsLive: other.Live}
			item.Sources = append(item.Sources, ps)
		}
		if len(item.Sources) == 0 {
			continue
		}
		item.SelectedSourceID = item.Sources[0].ID
		if ep.ID == in.EpisodeID {
			active = len(items)
		}
		items = append(items, item)
	}
	if active < 0 || len(items) == 0 {
		return nil, 0, errors.New("所选剧集已变更，请刷新详情")
	}
	return items, active, nil
}
func (s *Server) catalogPlaylistHTTP(w http.ResponseWriter, r *http.Request) {
	user, ok := s.catalogUser(w, r)
	if !ok {
		return
	}
	if !s.enforceRateLimit(w, r, "catalog-selection", user.ID, 20, time.Minute) {
		return
	}
	var in CatalogSelection
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	items, active, e := s.catalog.playlist(r.Context(), user, in)
	if e != nil {
		writeError(w, 400, e)
		return
	}
	writeJSON(w, 200, apiResponse{Data: map[string]any{"items": items, "active": active}})
}

func (s *Server) appendCatalogPlaylist(w http.ResponseWriter, r *http.Request) {
	user, ok := s.catalogUser(w, r)
	if !ok {
		return
	}
	if !s.enforceRateLimit(w, r, "catalog-selection", user.ID, 20, time.Minute) {
		return
	}
	var in struct {
		Version   int64            `json:"version"`
		Selection CatalogSelection `json:"selection"`
	}
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	room, f, e := s.authorizedRoom(r.Context(), code, user)
	if e != nil {
		writeRoomFeatureError(w, e)
		return
	}
	if !f.grants(room, user.ID).Playlist {
		writeError(w, 403, ErrForbidden)
		return
	}
	items, _, e := s.catalog.playlist(r.Context(), user, in.Selection)
	if e != nil {
		writeError(w, 400, e)
		return
	}
	// Catalog IDs and episode paths were generated by the server. Append the
	// complete selection in one CAS; do not probe every episode or partially save.
	e = s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		room, f, e := s.authorizedRoom(ctx, code, user)
		if e != nil {
			return e
		}
		if !f.grants(room, user.ID).Playlist {
			return ErrForbidden
		}
		if f.Version != in.Version {
			return ErrRoomConflict
		}
		if len(f.Playlist)+len(items) > 500 {
			return errors.New("房间片单最多 500 集，请仅加入当前集或整理片单")
		}
		next := 0
		for _, item := range f.Playlist {
			next = max(next, item.Episode+1)
		}
		for i := range items {
			items[i].Episode = next + i
		}
		f.Playlist = append(f.Playlist, items...)
		if f.ActiveItemID == "" {
			f.ActiveItemID = items[0].ID
			selection, e := s.selectionForItem(ctx, room, f, items[0], false)
			if e != nil {
				return e
			}
			return s.saveSelectedMedia(ctx, room, f, selection)
		}
		version := f.Version
		f.Version++
		if e = s.repo.SaveRoomFeatures(ctx, code, version, f, nil); e != nil {
			return e
		}
		return s.publishFeatureChange(ctx, room)
	})
	if e != nil {
		writeRoomFeatureError(w, e)
		return
	}
	s.getRoom(w, r)
}

// Only the owner can perform automatic recovery. CAS, expected source version,
// and episode identity make a second client's failure harmless after switching.
func (s *Server) recoverCatalogPlayback(w http.ResponseWriter, r *http.Request) {
	user, ok := s.catalogUser(w, r)
	if !ok {
		return
	}
	if !s.enforceRateLimit(w, r, "catalog-recover", user.ID, 12, time.Minute) {
		return
	}
	var in struct {
		Version       int64    `json:"version"`
		SourceVersion int64    `json:"source_version"`
		ItemID        string   `json:"item_id"`
		Attempted     []string `json:"attempted"`
	}
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	if len(in.Attempted) > 8 {
		writeError(w, 400, ErrForbidden)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	// Resolve candidate tickets before taking the distributed room lock. A slow
	// upstream must not outlive that lock's lease and race another room update.
	room, f, e := s.authorizedRoom(r.Context(), code, user)
	if e != nil {
		writeRoomFeatureError(w, e)
		return
	}
	if room.OwnerID != user.ID || !f.grants(room, user.ID).Playback {
		writeError(w, 403, ErrForbidden)
		return
	}
	if in.Version != f.Version || in.SourceVersion != room.Playback.SourceVersion || in.ItemID != f.ActiveItemID {
		writeRoomFeatureError(w, ErrRoomConflict)
		return
	}
	active, idx, exists := f.activeSource()
	if !exists || !strings.HasPrefix(active.MediaSourceID, "cat_") {
		writeError(w, 400, errors.New("当前片源不支持自动换源"))
		return
	}
	attempted := map[string]bool{active.ID: true}
	for _, id := range in.Attempted {
		attempted[id] = true
	}
	selected := ""
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	for _, candidate := range f.Playlist[idx].Sources {
		if attempted[candidate.ID] || !strings.HasPrefix(candidate.MediaSourceID, "cat_") {
			continue
		}
		if _, err := s.sources.PrepareMediaVariant(ctx, candidate.OwnerID, candidate.MediaSourceID, candidate.MediaPath, candidate.VariantID); err == nil {
			selected = candidate.ID
			break
		}
	}
	if selected == "" {
		writeError(w, 502, errors.New("当前集没有其他可用线路，请手动重试或选择片源"))
		return
	}
	e = s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		room, f, e := s.authorizedRoom(ctx, code, user)
		if e != nil {
			return e
		}
		if room.OwnerID != user.ID || !f.grants(room, user.ID).Playback {
			return ErrForbidden
		}
		if in.Version != f.Version || in.SourceVersion != room.Playback.SourceVersion || in.ItemID != f.ActiveItemID {
			return ErrRoomConflict
		}
		source, index, ok := f.activeSource()
		if !ok || !strings.HasPrefix(source.MediaSourceID, "cat_") {
			return errors.New("当前片源不支持自动换源")
		}
		item := f.Playlist[index]
		for _, candidate := range item.Sources {
			if candidate.ID != selected {
				continue
			}
			item.SelectedSourceID = candidate.ID
			playback := projected(room.Playback, time.Now())
			playback.SourceVersion++
			selection := RoomMediaSelection{MediaSourceID: candidate.MediaSourceID, MediaPath: candidate.MediaPath, Playback: playback}
			f.Playlist[index] = item
			return s.saveSelectedMedia(ctx, room, f, selection)
		}
		return errors.New("当前集没有其他可用线路，请手动重试或选择片源")
	})
	if e != nil {
		writeRoomFeatureError(w, e)
		return
	}
	s.getRoom(w, r)
}
