package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type CatalogFilm struct {
	ID         string         `json:"id"`
	SourceID   string         `json:"source_id"`
	SourceName string         `json:"source_name"`
	Title      string         `json:"title"`
	Year       string         `json:"year"`
	Kind       string         `json:"kind"`
	Remarks    string         `json:"remarks"`
	Summary    string         `json:"summary"`
	Actors     string         `json:"actors"`
	Poster     string         `json:"poster"`
	GroupKey   string         `json:"group_key"`
	Groups     []CatalogGroup `json:"groups,omitempty"`
}
type CatalogGroup struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Episodes []CatalogEpisode `json:"episodes"`
}
type CatalogEpisode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"-"`
	Live      bool   `json:"live,omitempty"`
	UserAgent string `json:"-"`
}
type CatalogSearchResult struct {
	Items   []CatalogFilm `json:"items"`
	Page    int           `json:"page"`
	HasMore bool          `json:"has_more"`
}

var catalogFilmID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
var catalogYear = regexp.MustCompile(`(?:19|20)[0-9]{2}`)
var catalogHTML = regexp.MustCompile(`<[^>]*>`)

func catalogPlain(raw string, n int) string {
	return catalogText(html.UnescapeString(catalogHTML.ReplaceAllString(raw, "")), n)
}
func catalogNormalize(raw string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, raw)
}
func catalogGroupKey(title, year, kind, source, id string) string {
	// Unknown years remain separate so remakes and seasons cannot merge silently.
	if year == "" {
		return catalogHash(source + ":" + id)
	}
	return catalogHash(catalogNormalize(title) + ":" + year + ":" + kind)
}
func (c *CatalogService) cms(ctx context.Context, src CatalogSource, query, id string, page int, refresh bool) (CatalogSearchResult, error) {
	u, e := catalogURL(src.URL)
	if e != nil {
		return CatalogSearchResult{}, e
	}
	q := u.Query()
	q.Set("ac", "detail")
	q.Set("pg", strconv.Itoa(page))
	if query != "" {
		q.Set("wd", query)
	}
	if id != "" {
		q.Set("ids", id)
	}
	u.RawQuery = q.Encode()
	key := catalogHash(u.String())
	c.mu.Lock()
	entry, ok := c.cache[key]
	c.mu.Unlock()
	var data []byte
	if ok && !refresh && entry.Expires.After(c.now()) {
		data = entry.Data
	} else {
		// The first request's bounded context owns the upstream request. Waiters can
		// cancel independently, and no unbounded goroutine/network pool is created.
		done := c.flight.DoChan(key, func() (any, error) {
			raw, e := c.fetch(ctx, u.String())
			if e != nil {
				return nil, e
			}
			if _, e = decodeCMS(raw); e != nil {
				return nil, e
			}
			c.mu.Lock()
			if len(c.cache) >= 128 {
				for k := range c.cache {
					delete(c.cache, k)
					break
				}
			}
			c.cache[key] = catalogCacheEntry{Data: raw, Expires: c.now().Add(2 * time.Minute)}
			c.mu.Unlock()
			return raw, nil
		})
		select {
		case v := <-done:
			if v.Err != nil {
				return CatalogSearchResult{}, v.Err
			}
			data = v.Val.([]byte)
		case <-ctx.Done():
			return CatalogSearchResult{}, ctx.Err()
		}
	}
	root, e := decodeCMS(data)
	if e != nil {
		return CatalogSearchResult{}, e
	}
	list := root["list"].([]any)
	items := []CatalogFilm{}
	for _, row := range list {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		film := c.film(src, m)
		if film.Title == "" || !catalogFilmID.MatchString(film.ID) {
			continue
		}
		if id != "" && id != film.ID {
			continue
		}
		items = append(items, film)
		if len(items) >= 100 {
			break
		}
	}
	count, _ := strconv.Atoi(anyString(root["pagecount"]))
	return CatalogSearchResult{Items: items, Page: page, HasMore: page < count}, nil
}
func decodeCMS(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var root map[string]any
	if dec.Decode(&root) != nil {
		return nil, errors.New("片源不是兼容的 JSON 影视接口")
	}
	list, ok := root["list"].([]any)
	if !ok || len(list) > 1000 {
		return nil, errors.New("接口缺少有效的影片列表")
	}
	if code := anyString(root["code"]); code != "" && code != "1" && code != "200" {
		return nil, errors.New("影视接口拒绝请求")
	}
	return root, nil
}
func (c *CatalogService) film(src CatalogSource, m map[string]any) CatalogFilm {
	title := catalogPlain(mapText(m, "vod_name"), 180)
	kind := "movie"
	typ := mapText(m, "type_name")
	if strings.Contains(typ, "剧") {
		kind = "tv"
	}
	if strings.Contains(typ, "动漫") || strings.Contains(typ, "动画") {
		kind = "anime"
	}
	if strings.Contains(typ, "综艺") {
		kind = "variety"
	}
	f := CatalogFilm{ID: mapText(m, "vod_id"), SourceID: src.ID, SourceName: src.Name, Title: title, Year: catalogYear.FindString(mapText(m, "vod_year")), Kind: kind, Remarks: catalogPlain(mapText(m, "vod_remarks"), 150), Summary: catalogPlain(mapText(m, "vod_content", "vod_blurb"), 1500), Actors: catalogPlain(mapText(m, "vod_actor"), 400), Groups: []CatalogGroup{}}
	f.GroupKey = catalogGroupKey(f.Title, f.Year, f.Kind, src.ID, f.ID)
	f.Poster = c.posterURL(mapText(m, "vod_pic"))
	names := strings.Split(mapText(m, "vod_play_from"), "$$$")
	groups := strings.Split(mapText(m, "vod_play_url"), "$$$")
	seenGroups := map[string]bool{}
	for i, raw := range groups {
		if i >= 8 {
			break
		}
		label := "线路 " + strconv.Itoa(i+1)
		if i < len(names) && names[i] != "" {
			label = catalogPlain(names[i], 60)
		}
		gid := catalogHash(label)
		if seenGroups[gid] {
			gid = catalogHash(label + ":" + strconv.Itoa(i))
		}
		seenGroups[gid] = true
		group := CatalogGroup{ID: gid, Name: label, Episodes: []CatalogEpisode{}}
		seen := map[string]bool{}
		for n, part := range strings.Split(raw, "#") {
			if n >= 500 {
				break
			}
			name, target, found := strings.Cut(part, "$")
			if !found {
				target = name
				name = "正片"
			}
			name = catalogPlain(name, 100)
			u, e := catalogURL(target)
			if e != nil || name == "" {
				continue
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			group.Episodes = append(group.Episodes, CatalogEpisode{ID: catalogHash(name), Name: name, URL: u.String()})
		}
		if len(group.Episodes) > 0 {
			f.Groups = append(f.Groups, group)
		}
	}
	return f
}
func channelFilm(src CatalogSource, ch CatalogChannel) CatalogFilm {
	return CatalogFilm{ID: ch.ID, SourceID: src.ID, SourceName: src.Name, Title: ch.Name, Kind: "live", Remarks: ch.Group, GroupKey: catalogHash(src.ID + ":" + ch.ID), Groups: []CatalogGroup{{ID: "live", Name: "直播", Episodes: []CatalogEpisode{{ID: "live", Name: "直播", URL: ch.URL, Live: true, UserAgent: ch.UserAgent}}}}}
}
func (c *CatalogService) search(ctx context.Context, src CatalogSource, query string, page int) (CatalogSearchResult, error) {
	if src.Kind == "cms" {
		res, e := c.cms(ctx, src, query, "", page, false)
		for i := range res.Items {
			res.Items[i].Groups = nil
		}
		return res, e
	}
	result := CatalogSearchResult{Items: []CatalogFilm{}, Page: page}
	matches := []CatalogChannel{}
	for _, ch := range src.Channels {
		if query == "" || strings.Contains(catalogNormalize(ch.Name+" "+ch.Group), catalogNormalize(query)) {
			matches = append(matches, ch)
		}
	}
	start := (page - 1) * 40
	end := min(start+40, len(matches))
	if start >= len(matches) {
		return result, nil
	}
	for _, ch := range matches[start:end] {
		f := channelFilm(src, ch)
		f.Groups = nil
		result.Items = append(result.Items, f)
	}
	result.HasMore = end < len(matches)
	return result, nil
}
func (c *CatalogService) detail(ctx context.Context, src CatalogSource, id string) (CatalogFilm, error) {
	if !catalogFilmID.MatchString(id) {
		return CatalogFilm{}, errors.New("影片标识无效")
	}
	if src.Kind == "cms" {
		res, e := c.cms(ctx, src, "", id, 1, false)
		if e != nil {
			return CatalogFilm{}, e
		}
		if len(res.Items) != 1 {
			return CatalogFilm{}, ErrNotFound
		}
		return res.Items[0], nil
	}
	for _, ch := range src.Channels {
		if ch.ID == id {
			return channelFilm(src, ch), nil
		}
	}
	return CatalogFilm{}, ErrNotFound
}
func (s *Server) catalogUser(w http.ResponseWriter, r *http.Request) (User, bool) {
	u, e := s.userFromRequest(r)
	if e != nil || !hasRegisteredAccount(u) {
		writeError(w, 401, ErrUnauthorized)
		return u, false
	}
	return u, true
}
func (s *Server) catalogSourcesHTTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.catalogUser(w, r); !ok {
		return
	}
	st, _, e := s.catalog.read(r.Context())
	if e != nil {
		writeError(w, 503, errors.New("片源目录暂不可用"))
		return
	}
	sources := []map[string]any{}
	for _, src := range catalogSources(st, true) {
		sources = append(sources, catalogPublicSource(src, st))
	}
	writeJSON(w, 200, apiResponse{Data: map[string]any{"sources": sources}})
}
func (s *Server) catalogSearchHTTP(w http.ResponseWriter, r *http.Request) {
	user, ok := s.catalogUser(w, r)
	if !ok {
		return
	}
	if !s.enforceRateLimit(w, r, "catalog-search", user.ID, 180, time.Minute) {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := 1
	var e error
	if p := r.URL.Query().Get("page"); p != "" {
		page, e = strconv.Atoi(p)
	}
	if e != nil || page < 1 || page > 100 || len([]rune(q)) > 100 {
		writeError(w, 400, errors.New("搜索词或页码无效"))
		return
	}
	id := r.URL.Query().Get("source_id")
	st, _, e := s.catalog.read(r.Context())
	if e != nil {
		writeError(w, 503, errors.New("片源目录暂不可用"))
		return
	}
	if st.Health[id].SuspendedUntil > s.catalog.now().UnixMilli() {
		writeError(w, 503, errors.New("片源正在冷却，稍后将自动恢复检测"))
		return
	}
	src, e := s.catalog.source(r.Context(), id)
	if e != nil {
		writeError(w, 404, e)
		return
	}
	started := s.catalog.now()
	res, e := s.catalog.search(r.Context(), src, q, page)
	s.catalog.health(r.Context(), id, started, e)
	if e != nil {
		writeError(w, 502, e)
		return
	}
	writeJSON(w, 200, apiResponse{Data: res})
}
func (s *Server) catalogDetailHTTP(w http.ResponseWriter, r *http.Request) {
	user, ok := s.catalogUser(w, r)
	if !ok {
		return
	}
	if !s.enforceRateLimit(w, r, "catalog-detail", user.ID, 90, time.Minute) {
		return
	}
	src, e := s.catalog.source(r.Context(), r.URL.Query().Get("source_id"))
	if e != nil {
		writeError(w, 404, e)
		return
	}
	f, e := s.catalog.detail(r.Context(), src, r.URL.Query().Get("id"))
	if e != nil {
		writeError(w, 502, e)
		return
	}
	writeJSON(w, 200, apiResponse{Data: f})
}

type catalogPoster struct {
	URL     string `json:"url"`
	Expires int64  `json:"expires"`
}

func (c *CatalogService) posterURL(raw string) string {
	if c.vault == nil {
		return ""
	}
	if _, e := catalogURL(raw); e != nil {
		return ""
	}
	data, _ := json.Marshal(catalogPoster{URL: raw, Expires: c.now().Add(24 * time.Hour).Unix()})
	token, e := c.vault.Encrypt(data, "catalog-poster")
	if e != nil {
		return ""
	}
	return "/api/v1/catalog/poster?token=" + url.QueryEscape(token)
}
func (s *Server) catalogPosterHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if len(token) > 12000 || s.catalog.vault == nil {
		writeError(w, 400, ErrForbidden)
		return
	}
	raw, e := s.catalog.vault.Decrypt(token, "catalog-poster")
	var p catalogPoster
	if e != nil || json.Unmarshal(raw, &p) != nil || p.Expires < s.catalog.now().Unix() {
		writeError(w, 403, ErrForbidden)
		return
	}
	data, e := s.catalog.fetch(r.Context(), p.URL)
	if e != nil {
		writeError(w, 502, errors.New("海报暂不可用"))
		return
	}
	mime := http.DetectContentType(data)
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" && mime != "image/gif" {
		writeError(w, 415, ErrForbidden)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=1800")
	_, _ = io.Copy(w, bytes.NewReader(data))
}
