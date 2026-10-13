package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) registerCatalogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/catalog", s.adminCatalog)
	mux.HandleFunc("POST /api/v1/admin/catalog/preview", s.adminCatalogPreview)
	mux.HandleFunc("POST /api/v1/admin/catalog/commit", s.adminCatalogCommit)
	mux.HandleFunc("POST /api/v1/admin/catalog/manage", s.adminCatalogManage)
	mux.HandleFunc("POST /api/v1/admin/catalog/probe", s.adminCatalogProbe)
	mux.HandleFunc("GET /api/v1/catalog/sources", s.catalogSourcesHTTP)
	mux.HandleFunc("GET /api/v1/catalog/search", s.catalogSearchHTTP)
	mux.HandleFunc("GET /api/v1/catalog/detail", s.catalogDetailHTTP)
	mux.HandleFunc("GET /api/v1/catalog/poster", s.catalogPosterHTTP)
	mux.HandleFunc("POST /api/v1/catalog/playlist", s.catalogPlaylistHTTP)
	mux.HandleFunc("POST /api/v1/rooms/{code}/playback/recover", s.recoverCatalogPlayback)
}
func catalogDecode(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 3<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return errors.New("请求格式无效或内容超过 3 MB")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("请求包含多余数据")
	}
	return nil
}
func catalogPublicSource(src CatalogSource, st CatalogState) map[string]any {
	enabled := false
	for _, sub := range st.Subscriptions {
		if sub.Enabled {
			for _, item := range sub.Sources {
				if item.ID == src.ID {
					enabled = true
				}
			}
		}
	}
	return map[string]any{"id": src.ID, "name": src.Name, "kind": src.Kind, "enabled": enabled && !st.Disabled[src.ID], "health": st.Health[src.ID], "channels": len(src.Channels)}
}
func (s *Server) adminCatalog(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "source.manage"); !ok {
		return
	}
	st, _, e := s.catalog.read(r.Context())
	if e != nil {
		writeAdminError(w, e)
		return
	}
	subs := []map[string]any{}
	for _, sub := range st.Subscriptions {
		subs = append(subs, map[string]any{"id": sub.ID, "name": sub.Name, "remote": sub.URL != "", "enabled": sub.Enabled, "interval_hours": sub.IntervalHours, "last_run": sub.LastRun, "last_success": sub.LastSuccess, "next_run": sub.NextRun, "error": sub.Error, "sources": len(sub.Sources)})
	}
	srcs := []map[string]any{}
	for _, src := range catalogSources(st, false) {
		srcs = append(srcs, catalogPublicSource(src, st))
	}
	writeJSON(w, 200, apiResponse{Data: map[string]any{"revision": st.Revision, "subscriptions": subs, "sources": srcs, "history": st.History}})
}

type catalogPreviewInput struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	Content       string `json:"content"`
	IntervalHours int    `json:"interval_hours"`
}

func (c *CatalogService) preview(ctx context.Context, actor string, in catalogPreviewInput) (string, CatalogUpdate, error) {
	st, _, e := c.read(ctx)
	if e != nil {
		return "", CatalogUpdate{}, e
	}
	sub := CatalogSubscription{ID: in.ID, Name: strings.TrimSpace(in.Name), URL: strings.TrimSpace(in.URL), Enabled: true, IntervalHours: in.IntervalHours}
	old := []CatalogSource{}
	if in.ID != "" {
		found := false
		for _, current := range st.Subscriptions {
			if current.ID == in.ID {
				sub = current
				old = current.Sources
				found = true
				break
			}
		}
		if !found {
			return "", CatalogUpdate{}, ErrNotFound
		}
		if in.Name != "" {
			sub.Name = strings.TrimSpace(in.Name)
		}
		if in.URL != "" {
			sub.URL = strings.TrimSpace(in.URL)
		}
		if in.IntervalHours > 0 {
			sub.IntervalHours = in.IntervalHours
		}
	} else {
		sub.ID = mustRandomString(12)
	}
	if len([]rune(sub.Name)) < 1 || len([]rune(sub.Name)) > 64 {
		return "", CatalogUpdate{}, errors.New("订阅名称需要 1–64 个字")
	}
	if sub.IntervalHours == 0 {
		sub.IntervalHours = 24
	}
	if sub.IntervalHours < 1 || sub.IntervalHours > 168 {
		return "", CatalogUpdate{}, errors.New("更新间隔需要 1–168 小时")
	}
	if in.Content != "" && in.URL != "" {
		return "", CatalogUpdate{}, errors.New("请选择订阅地址或上传内容其中一种")
	}
	data := []byte(in.Content)
	if len(data) == 0 {
		if sub.URL == "" {
			return "", CatalogUpdate{}, errors.New("请填写订阅地址或导入内容")
		}
		data, e = c.fetch(ctx, sub.URL)
		if e != nil {
			return "", CatalogUpdate{}, e
		}
	} else {
		sub.URL = ""
	}
	parsed, skips, dups, e := c.parseImport(ctx, data, sub.URL, sub.Name)
	if e != nil {
		return "", CatalogUpdate{}, e
	}
	sub.Sources = parsed
	u := catalogDiff(old, parsed)
	u.At = c.now().UnixMilli()
	u.SubscriptionID = sub.ID
	u.Name = sub.Name
	u.Mode = "manual"
	u.Skipped = skips
	u.Duplicates = dups
	existing := map[string]bool{}
	for _, other := range st.Subscriptions {
		if other.ID != sub.ID {
			for _, src := range other.Sources {
				existing[src.ID] = true
			}
		}
	}
	for _, src := range parsed {
		if existing[src.ID] {
			u.Duplicates++
		}
		existing[src.ID] = true
	}
	if len(existing) > catalogMaxSources {
		return "", u, errors.New("全站最多 64 个独立片源，请先删除不用的订阅")
	}
	if len(st.Subscriptions) >= 24 && in.ID == "" {
		return "", u, errors.New("最多 24 份订阅")
	}
	token := mustRandomString(24)
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, p := range c.previews {
		if !p.Expires.After(c.now()) {
			delete(c.previews, id)
		}
	}
	if len(c.previews) >= 24 {
		return "", u, errors.New("预览队列已满，请稍后重试")
	}
	c.previews[token] = catalogPreview{Actor: actor, Expires: c.now().Add(10 * time.Minute), Revision: st.Revision, Subscription: sub, Update: u}
	return token, u, nil
}
func (s *Server) adminCatalogPreview(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "source.manage")
	if !ok {
		return
	}
	if !s.enforceRateLimit(w, r, "catalog-import", actor.ID, 15, time.Minute) {
		return
	}
	var in catalogPreviewInput
	if e := catalogDecode(w, r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	token, update, e := s.catalog.preview(ctx, actor.ID, in)
	if e != nil {
		writeError(w, 400, e)
		return
	}
	writeJSON(w, 200, apiResponse{Data: map[string]any{"token": token, "expires_in": 600, "update": update}})
}
func (c *CatalogService) commit(ctx context.Context, actor, token string) error {
	c.mu.Lock()
	p, ok := c.previews[token]
	c.mu.Unlock()
	if !ok || p.Actor != actor || !p.Expires.After(c.now()) {
		return errors.New("预览已失效，请重新生成预览")
	}
	e := c.mutate(ctx, actor, "import", func(st *CatalogState) error {
		if st.Revision != p.Revision {
			return ErrAdminConflict
		}
		sub := p.Subscription
		sub.Generation++
		sub.LastRun = c.now().UnixMilli()
		sub.LastSuccess = sub.LastRun
		sub.NextRun = c.now().Add(time.Duration(sub.IntervalHours) * time.Hour).UnixMilli()
		sub.Error = ""
		sub.Lease = ""
		sub.LeaseUntil = 0
		found := false
		for i, v := range st.Subscriptions {
			if v.ID == sub.ID {
				st.Subscriptions[i] = sub
				found = true
				break
			}
		}
		if !found {
			st.Subscriptions = append(st.Subscriptions, sub)
		}
		st.Revision++
		p.Update.At = c.now().UnixMilli()
		appendCatalogUpdate(st, p.Update)
		return nil
	})
	if e == nil {
		c.mu.Lock()
		delete(c.previews, token)
		c.mu.Unlock()
	}
	return e
}
func (s *Server) adminCatalogCommit(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "source.manage")
	if !ok {
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	if e := s.catalog.commit(r.Context(), actor.ID, in.Token); e != nil {
		writeAdminError(w, e)
		return
	}
	s.adminCatalog(w, r)
}
func (s *Server) adminCatalogManage(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "source.manage")
	if !ok {
		return
	}
	var in struct {
		Revision      int64    `json:"revision"`
		Action        string   `json:"action"`
		IDs           []string `json:"ids"`
		Enabled       bool     `json:"enabled"`
		IntervalHours int      `json:"interval_hours"`
	}
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 64 {
		writeError(w, 400, errors.New("请选择 1–64 项"))
		return
	}
	e := s.catalog.mutate(r.Context(), actor.ID, in.Action, func(st *CatalogState) error {
		if st.Revision != in.Revision {
			return ErrAdminConflict
		}
		ids := map[string]bool{}
		for _, id := range in.IDs {
			ids[id] = true
		}
		switch in.Action {
		case "source-toggle":
			for _, src := range catalogSources(*st, false) {
				if ids[src.ID] {
					st.Disabled[src.ID] = !in.Enabled
					delete(ids, src.ID)
				}
			}
		case "subscription-toggle", "delete", "schedule":
			next := []CatalogSubscription{}
			for _, sub := range st.Subscriptions {
				if ids[sub.ID] {
					delete(ids, sub.ID)
					sub.Generation++
					sub.Lease = ""
					switch in.Action {
					case "delete":
						continue
					case "subscription-toggle":
						sub.Enabled = in.Enabled
					case "schedule":
						if in.IntervalHours < 1 || in.IntervalHours > 168 {
							return errors.New("更新间隔需要 1–168 小时")
						}
						sub.IntervalHours = in.IntervalHours
						sub.NextRun = s.catalog.now().Add(time.Duration(in.IntervalHours) * time.Hour).UnixMilli()
					}
				}
				next = append(next, sub)
			}
			st.Subscriptions = next
		default:
			return errors.New("不支持的订阅操作")
		}
		if len(ids) > 0 {
			return ErrNotFound
		}
		st.Revision++
		return nil
	})
	if e != nil {
		writeAdminError(w, e)
		return
	}
	s.adminCatalog(w, r)
}
func (s *Server) adminCatalogProbe(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "source.manage")
	if !ok {
		return
	}
	if !s.enforceRateLimit(w, r, "catalog-probe", actor.ID, 15, time.Minute) {
		return
	}
	var in struct {
		IDs []string `json:"ids"`
	}
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 8 {
		writeError(w, 400, errors.New("每批可检测 1–8 个片源"))
		return
	}
	type result struct {
		ID    string `json:"id"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	ch := make(chan result, len(in.IDs))
	for _, id := range in.IDs {
		go func(id string) {
			src, e := s.catalog.source(r.Context(), id)
			started := s.catalog.now()
			if e == nil {
				if src.Kind == "cms" {
					_, e = s.catalog.cms(r.Context(), src, "", "", 1, true)
				} else {
					if len(src.Channels) == 0 {
						e = ErrNotFound
					} else {
						_, e = s.catalog.probeMedia(r.Context(), src.Channels[0].URL)
					}
				}
				s.catalog.health(r.Context(), id, started, e)
			}
			v := result{ID: id, OK: e == nil}
			if e != nil {
				v.Error = "连接失败或接口不兼容"
			}
			ch <- v
		}(id)
	}
	out := []result{}
	for range in.IDs {
		out = append(out, <-ch)
	}
	writeJSON(w, 200, apiResponse{Data: map[string]any{"results": out}})
}

func (c *CatalogService) refreshDue(ctx context.Context) error {
	st, _, e := c.read(ctx)
	if e != nil {
		return e
	}
	for _, sub := range st.Subscriptions {
		now := c.now().UnixMilli()
		if !sub.Enabled || sub.URL == "" || sub.NextRun > now || sub.LeaseUntil > now {
			continue
		}
		lease := mustRandomString(12)
		e = c.mutate(ctx, "", "claim", func(current *CatalogState) error {
			for i, v := range current.Subscriptions {
				if v.ID == sub.ID && v.Generation == sub.Generation && v.Enabled && v.NextRun <= now && v.LeaseUntil <= now {
					current.Subscriptions[i].Lease = lease
					current.Subscriptions[i].LeaseUntil = c.now().Add(2 * time.Minute).UnixMilli()
					return nil
				}
			}
			return ErrNotFound
		})
		if e != nil {
			continue
		}
		fetchCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
		data, fetchErr := c.fetch(fetchCtx, sub.URL)
		var sources []CatalogSource
		var skips []string
		var dups int
		if fetchErr == nil {
			sources, skips, dups, fetchErr = c.parseImport(fetchCtx, data, sub.URL, sub.Name)
		}
		cancel()
		e = c.mutate(ctx, "", "sync", func(current *CatalogState) error {
			for i, v := range current.Subscriptions {
				if v.ID != sub.ID || v.Generation != sub.Generation || v.Lease != lease {
					continue
				}
				update := catalogDiff(v.Sources, sources)
				update.At = c.now().UnixMilli()
				update.SubscriptionID = v.ID
				update.Name = v.Name
				update.Mode = "scheduled"
				update.Skipped = skips
				update.Duplicates = dups
				if fetchErr == nil {
					candidate := *current
					candidate.Subscriptions = append([]CatalogSubscription(nil), current.Subscriptions...)
					candidate.Subscriptions[i].Sources = sources
					if len(catalogSources(candidate, false)) > catalogMaxSources {
						fetchErr = errors.New("更新超过 64 个独立片源的限制")
					}
				}
				if fetchErr != nil {
					v.Error = fetchErr.Error()
					update.Error = v.Error
					update.Added = 0
					update.Changed = 0
					update.Removed = 0
					update.Changes = []CatalogChange{}
				} else {
					v.Sources = sources
					v.LastSuccess = update.At
					v.Error = ""
					v.Generation++
					current.Revision++
				}
				v.LastRun = update.At
				v.NextRun = c.now().Add(time.Duration(v.IntervalHours) * time.Hour).UnixMilli()
				v.Lease = ""
				v.LeaseUntil = 0
				current.Subscriptions[i] = v
				appendCatalogUpdate(current, update)
				return nil
			}
			return ErrNotFound
		})
		if e != nil && !errors.Is(e, ErrNotFound) {
			return e
		}
	}
	return nil
}
func (s *Server) catalogLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if e := s.catalog.refreshDue(ctx); e != nil && ctx.Err() == nil {
			s.options.Logger.Warn("catalog scheduled update unavailable")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
