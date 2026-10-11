package app

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
)

func (r *MemoryRepository) AdminReadSource(_ context.Context, id string) (ManagedSource, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	source, ok := r.mediaSources[id]
	if !ok {
		return ManagedSource{}, ErrNotFound
	}
	return managedSource(source, r.usersByID[source.UserID].User), nil
}
func (r *PostgresRepository) AdminReadSource(ctx context.Context, id string) (ManagedSource, error) {
	return scanManagedSource(r.pool.QueryRow(ctx, `SELECT `+managedSourceColumns+` FROM media_sources s JOIN users u ON u.id=s.user_id WHERE s.id=$1`, id))
}
func (s *Server) adminSources(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "source.manage"); !ok {
		return
	}
	q := adminQuery(r)
	items, total, err := s.repo.AdminFindSources(r.Context(), q)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	ready := s.sources != nil
	parser := ready && s.sources.platform.available()
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"sources": items, "total": total, "offset": q.Offset, "limit": q.Limit, "has_more": q.Offset+len(items) < total, "media_service_ready": ready, "platform_parser_ready": parser}})
}
func (s *Server) adminChangeSource(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "source.manage")
	if !ok {
		return
	}
	var in struct {
		AdminSourceChange
		Confirm string `json:"confirm"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Version < 1 || !slices.Contains([]string{"rename", "enable", "disable", "delete"}, in.Action) || !safeModerationText(in.Reason, 500, true) || (in.Action == "rename" && !safeModerationText(in.Name, 64, false)) {
		writeError(w, 400, errors.New("媒体来源操作参数无效"))
		return
	}
	if in.Action == "delete" && in.Confirm != "DELETE SOURCE" {
		writeError(w, 428, errors.New("请确认删除媒体来源，关联播放可能中断"))
		return
	}
	item, err := s.repo.AdminMutateSource(r.Context(), r.PathValue("id"), actor.ID, in.AdminSourceChange)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: item})
}
func (s *Server) adminProbeSource(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "source.manage")
	if !ok {
		return
	}
	if s.sources == nil {
		writeError(w, 503, errors.New("媒体来源服务尚未配置"))
		return
	}
	if !s.enforceRateLimit(w, r, "admin-source-probe", actor.ID, 20, time.Minute) {
		return
	}
	item, err := s.repo.AdminReadSource(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	if item.Disabled {
		writeError(w, 409, errors.New("请先启用此媒体来源"))
		return
	}
	var in struct {
		URL string `json:"url"`
	}
	if err = decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	start := time.Now()
	count := 0
	if _, platform := platformDefinitions[item.Type]; platform {
		if in.URL == "" {
			writeError(w, 400, errors.New("检测视频平台时需填写该平台的视频链接"))
			return
		}
		files, e := s.sources.ResolvePlatform(ctx, item.OwnerID, item.ID, in.URL, 0)
		err = e
		count = len(files)
	} else {
		files, e := s.sources.Browse(ctx, item.OwnerID, item.ID, "/")
		err = e
		count = len(files)
	}
	success := err == nil
	if _, auditErr := s.repo.AppendAudit(r.Context(), AuditEvent{ActorID: actor.ID, Action: "source.probe", TargetType: "source", TargetID: item.ID, Metadata: map[string]any{"success": success}}); auditErr != nil {
		writeAdminError(w, auditErr)
		return
	}
	message := "连接成功"
	if !success {
		message = "连接失败，请检查来源服务、网络与凭据"
	}
	// Upstream errors can contain signed URLs. Only return a bounded status, never credentials or upstream error text.
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"success": success, "message": message, "elapsed_ms": time.Since(start).Milliseconds(), "items": count}})
}
