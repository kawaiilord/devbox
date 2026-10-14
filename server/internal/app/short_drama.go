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
	"regexp"
	"strconv"
	"strings"
	"time"
)

const guoguoSourceID = "builtin-guoguo"

var shortDramaID = regexp.MustCompile(`^[0-9]{1,32}$`)

type GuoguoService struct {
	base   *url.URL
	token  string
	client *http.Client
}

func NewGuoguoService(raw, token string) (*GuoguoService, error) {
	u, e := url.Parse(raw)
	if e != nil || u == nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") || len(token) < 32 {
		return nil, errors.New("果果短剧服务地址或令牌配置无效")
	}
	u.Path = ""
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &GuoguoService{base: u, token: token, client: &http.Client{Transport: transport, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("短剧服务不允许重定向") }}}, nil
}
func (g *GuoguoService) request(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	if g == nil {
		return nil, errors.New("果果短剧服务尚未启用")
	}
	u := *g.base
	u.Path = path
	u.RawQuery = query.Encode()
	var data []byte
	if body != nil {
		var e error
		data, e = json.Marshal(body)
		if e != nil {
			return nil, e
		}
	}
	r, e := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	r.Header.Set("Authorization", "Bearer "+g.token)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	res, e := g.client.Do(r)
	if e != nil {
		return nil, errors.New("短剧服务暂时连接失败，请重试")
	}
	return res, nil
}
func (g *GuoguoService) json(ctx context.Context, method, path string, q url.Values, body any) (map[string]any, error) {
	res, e := g.request(ctx, method, path, q, body)
	if e != nil {
		return nil, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("短剧数据暂不可用，请稍后重试")
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, 4<<20+1))
	if e != nil || len(b) > 4<<20 {
		return nil, errors.New("短剧数据读取失败")
	}
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if dec.Decode(&out) != nil || out == nil {
		return nil, errors.New("短剧数据格式无效")
	}
	return out, nil
}
func guoguoString(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return ""
	}
}
func guoguoFirst(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := guoguoString(row[key]); v != "" {
			return v
		}
	}
	return ""
}

type shortDramaQuery struct {
	Section string   `json:"section"`
	Query   string   `json:"query"`
	Offset  int      `json:"offset"`
	Session string   `json:"session"`
	Seen    []string `json:"seen"`
	Page    int      `json:"page"`
}

func (q *shortDramaQuery) validate() error {
	switch q.Section {
	case "short_play", "comic_series", "ai_series", "hot", "real", "comic", "ai", "search":
	default:
		return errors.New("短剧分类无效")
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.Page < 1 || q.Page > 500 || q.Offset < 0 || q.Offset > 1000000 || len(q.Session) > 4096 || strings.ContainsAny(q.Session, "\x00\r\n") || len(q.Seen) > 540 || len([]rune(q.Query)) > 80 || strings.ContainsAny(q.Query, "\x00\r\n") || q.Section == "search" && strings.TrimSpace(q.Query) == "" {
		return errors.New("短剧搜索或分页参数无效")
	}
	for _, id := range q.Seen {
		if !shortDramaID.MatchString(id) {
			return errors.New("短剧分页标识无效")
		}
	}
	return nil
}
func (c *CatalogService) guoguoFilm(row map[string]any) CatalogFilm {
	id := strings.TrimPrefix(guoguoFirst(row, "id", "sourceId"), "hongguo:")
	f := CatalogFilm{ID: id, SourceID: guoguoSourceID, SourceName: "果果短剧", Title: catalogPlain(guoguoFirst(row, "title", "name"), 200), Kind: "short", Summary: catalogPlain(guoguoFirst(row, "desc", "intro"), 3000), Groups: []CatalogGroup{}}
	f.GroupKey = catalogHash(guoguoSourceID + ":" + id)
	count := guoguoFirst(row, "totalEpisode", "total_episode", "chapterCount", "chapter_count", "episodeCount", "episode_count", "total")
	f.Remarks = catalogPlain(guoguoFirst(row, "category", "categoryName", "typeName", "channelName"), 60)
	if n, e := strconv.Atoi(count); e == nil && n > 0 {
		if f.Remarks != "" {
			f.Remarks += " · "
		}
		f.Remarks += fmt.Sprintf("%d 集", n)
	}
	cover := guoguoFirst(row, "coverUrl", "cover_url", "cover", "image", "poster")
	if strings.HasPrefix(cover, "/api/ui/image?") {
		u, _ := url.Parse(cover)
		cover = u.Query().Get("url")
	}
	if c.vault != nil && cover != "" && len(cover) < 8192 {
		data, _ := json.Marshal(catalogPoster{URL: cover, Expires: c.now().Add(24 * time.Hour).Unix()})
		if token, e := c.vault.Encrypt(data, "guoguo-poster"); e == nil {
			f.Poster = "/api/v1/short-drama/poster?token=" + url.QueryEscape(token)
		}
	}
	return f
}
func (c *CatalogService) guoguoDetail(ctx context.Context, id string) (CatalogFilm, error) {
	if !shortDramaID.MatchString(id) {
		return CatalogFilm{}, errors.New("短剧标识无效")
	}
	data, e := c.guoguo.json(ctx, "GET", "/bridge/detail", url.Values{"id": {id}}, nil)
	if e != nil {
		return CatalogFilm{}, e
	}
	row, _ := data["drama"].(map[string]any)
	f := c.guoguoFilm(row)
	if f.ID != id || f.Title == "" {
		return CatalogFilm{}, errors.New("短剧详情与所选剧集不一致")
	}
	group := CatalogGroup{ID: "main", Name: "果果短剧", Episodes: []CatalogEpisode{}}
	seen := map[string]bool{}
	rows, _ := data["episodes"].([]any)
	for _, v := range rows {
		ep, _ := v.(map[string]any)
		eid := guoguoString(ep["id"])
		if !shortDramaID.MatchString(eid) || seen[eid] {
			continue
		}
		seen[eid] = true
		group.Episodes = append(group.Episodes, CatalogEpisode{ID: eid, Name: catalogText(guoguoString(ep["name"]), 80)})
	}
	if len(group.Episodes) > 0 {
		f.Groups = append(f.Groups, group)
	}
	return f, nil
}
func (s *Server) shortDramaFeed(w http.ResponseWriter, r *http.Request) {
	user, ok := s.catalogUser(w, r)
	if !ok {
		return
	}
	if !s.enforceRateLimit(w, r, "short-drama-feed", user.ID, 50, time.Minute) {
		return
	}
	if s.catalog.guoguo == nil {
		writeError(w, 503, errors.New("果果短剧服务尚未启用"))
		return
	}
	var q shortDramaQuery
	if e := decodeJSON(r, &q); e != nil {
		writeError(w, 400, e)
		return
	}
	if e := q.validate(); e != nil {
		writeError(w, 400, e)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	var data map[string]any
	var e error
	if q.Section == "search" {
		data, e = s.catalog.guoguo.json(ctx, "GET", "/bridge/search", url.Values{"q": {strings.TrimSpace(q.Query)}}, nil)
	} else if q.Section == "hot" || q.Section == "real" || q.Section == "comic" || q.Section == "ai" {
		data, e = s.catalog.guoguo.json(ctx, "GET", "/bridge/rankings", url.Values{"board": {"hongguo-" + q.Section}, "page": {strconv.Itoa(q.Page)}}, nil)
	} else {
		data, e = s.catalog.guoguo.json(ctx, "POST", "/bridge/recommendations", nil, map[string]any{"genre": q.Section, "offset": q.Offset, "sessionId": q.Session, "seen": q.Seen})
	}
	if e != nil {
		writeError(w, 502, e)
		return
	}
	rows, _ := data["data"].([]any)
	ranked := false
	if rows == nil {
		rows, _ = data["items"].([]any)
		ranked = true
	}
	items := []CatalogFilm{}
	seen := map[string]bool{}
	for _, v := range rows {
		row, _ := v.(map[string]any)
		rank := ""
		if ranked {
			rank = guoguoString(row["rank"])
			row, _ = row["drama"].(map[string]any)
		}
		f := s.catalog.guoguoFilm(row)
		if !shortDramaID.MatchString(f.ID) || f.Title == "" || seen[f.ID] {
			continue
		}
		seen[f.ID] = true
		if rank != "" {
			f.Remarks = "热播榜第 " + rank + " 名 · " + f.Remarks
		}
		items = append(items, f)
	}
	more, _ := data["hasMore"].(bool)
	offset, _ := strconv.Atoi(guoguoString(data["nextOffset"]))
	if q.Section == "search" {
		more = false
	}
	writeJSON(w, 200, apiResponse{Data: map[string]any{"items": items, "has_more": more, "offset": offset, "session": guoguoString(data["sessionId"]), "page": q.Page, "warning": catalogText(guoguoString(data["warning"]), 200)}})
}
func (s *Server) shortDramaPoster(w http.ResponseWriter, r *http.Request) {
	c := s.catalog
	token := r.URL.Query().Get("token")
	if c.vault == nil || c.guoguo == nil || len(token) > 14000 {
		writeError(w, 403, ErrForbidden)
		return
	}
	b, e := c.vault.Decrypt(token, "guoguo-poster")
	var p catalogPoster
	if e != nil || json.Unmarshal(b, &p) != nil || p.Expires < c.now().Unix() {
		writeError(w, 403, ErrForbidden)
		return
	}
	res, e := c.guoguo.request(r.Context(), "GET", "/bridge/image", url.Values{"url": {p.URL}}, nil)
	if e != nil {
		writeError(w, 502, errors.New("海报暂不可用"))
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "image/") {
		w.WriteHeader(502)
		return
	}
	b, e = io.ReadAll(io.LimitReader(res.Body, 8<<20+1))
	if e != nil || len(b) > 8<<20 {
		w.WriteHeader(502)
		return
	}
	w.Header().Set("Content-Type", res.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}

// Media access is selected by a server-owned source binding, never by a URL
// imported from a subscription. Only this fixed service and its two media paths
// are reachable; redirects and unrelated private addresses remain prohibited.
func (g *GuoguoService) mediaURL(id, episode string) string {
	u := *g.base
	u.Path = "/bridge/stream.m3u8"
	u.RawQuery = url.Values{"id": {id}, "episode": {episode}}.Encode()
	return u.String()
}
func (g *GuoguoService) openMedia(ctx context.Context, target, method, rangeHeader, ifRange string) (*http.Response, error) {
	if g == nil {
		return nil, ErrForbidden
	}
	u, e := url.Parse(target)
	if e != nil || u.User != nil || u.Fragment != "" || u.Scheme != g.base.Scheme || u.Host != g.base.Host || (u.Path != "/bridge/stream.m3u8" && u.Path != "/bridge/segment.ts") || len(target) > 8192 || (method != "GET" && method != "HEAD") {
		return nil, ErrForbidden
	}
	r, e := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if e != nil {
		return nil, ErrForbidden
	}
	r.Header.Set("Authorization", "Bearer "+g.token)
	if rangeHeader != "" {
		r.Header.Set("Range", rangeHeader)
	}
	if ifRange != "" {
		r.Header.Set("If-Range", ifRange)
	}
	res, e := g.client.Do(r)
	if e != nil {
		return nil, errors.New("短剧播放连接失败")
	}
	return res, nil
}
