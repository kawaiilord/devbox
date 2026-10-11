package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

func adminQuery(r *http.Request) AdminQuery {
	values := r.URL.Query()
	offset, _ := strconv.Atoi(values.Get("offset"))
	limit, _ := strconv.Atoi(values.Get("limit"))
	if limit < 1 {
		limit = 25
	}
	return AdminQuery{Search: strings.TrimSpace(values.Get("q")), Status: values.Get("status"), OwnerID: values.Get("owner_id"), Kind: values.Get("kind"), Category: values.Get("category"), Offset: min(max(offset, 0), 1000000), Limit: min(limit, 100)}
}
func writeAdminError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrAdminConflict) || errors.Is(err, ErrRoomConflict) {
		writeError(w, 409, err)
		return
	}
	if errors.Is(err, ErrEmailExists) {
		writeError(w, 409, err)
		return
	}
	writeStoreError(w, err)
}
func (s *Server) adminSession(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	if !user.EmailVerified || len(rolePermissions[adminRole(user)]) == 0 {
		writeError(w, 403, ErrForbidden)
		return
	}
	permissions := []string{}
	for p, enabled := range rolePermissions[adminRole(user)] {
		if enabled {
			permissions = append(permissions, p)
		}
	}
	sort.Strings(permissions)
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"user": user, "role": adminRole(user), "permissions": permissions}})
}
func (s *Server) adminManagedUsers(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "user.view")
	if !ok {
		return
	}
	q := adminQuery(r)
	items, total, err := s.repo.AdminFindUsers(r.Context(), q)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	manageable := map[string]bool{}
	for _, item := range items {
		manageable[item.ID] = canManageAccount(actor, User{ID: item.ID, AdminRole: item.AdminRole})
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"users": items, "total": total, "offset": q.Offset, "limit": q.Limit, "has_more": q.Offset+len(items) < total, "can_manage": manageable}})
}
func (s *Server) adminCreateManagedUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "user.manage")
	if !ok {
		return
	}
	var in struct {
		Email         string `json:"email"`
		DisplayName   string `json:"display_name"`
		Password      string `json:"password"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	email, err := validateCredentials(in.Email, in.DisplayName, in.Password)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	item, err := s.repo.AdminCreateUser(r.Context(), AccountRecord{User: User{ID: mustRandomString(12), Email: email, DisplayName: strings.TrimSpace(in.DisplayName), EmailVerified: in.EmailVerified, SessionVersion: 1}, PasswordHash: hash, CreatedAt: time.Now()}, actor.ID)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, 201, apiResponse{Code: 0, Data: item})
}
func validateAdminUserChange(change AdminUserChange, password string) error {
	if change.Version < 1 {
		return errors.New("缺少记录版本，请刷新用户列表")
	}
	if !safeModerationText(change.Reason, 500, true) {
		return errors.New("操作原因无效")
	}
	switch change.Action {
	case "profile":
		if len([]rune(strings.TrimSpace(change.DisplayName))) < 2 || len([]rune(strings.TrimSpace(change.DisplayName))) > 32 {
			return errors.New("昵称需为 2–32 个字符")
		}
	case "password":
		if len(password) < 10 || len(password) > 128 {
			return errors.New("密码需为 10–128 字节")
		}
	case "role":
		if !slices.Contains([]string{"", "super_admin", "admin", "operator", "seller"}, change.Role) {
			return errors.New("账号角色无效")
		}
	case "ban", "delete":
		if strings.TrimSpace(change.Reason) == "" {
			return errors.New("请填写操作原因")
		}
	case "unban", "restore":
	default:
		return errors.New("不支持的账号操作")
	}
	return nil
}
func (s *Server) adminChangeUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "user.manage")
	if !ok {
		return
	}
	var in struct {
		AdminUserChange
		Password string `json:"password"`
		Confirm  string `json:"confirm"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if err := validateAdminUserChange(in.AdminUserChange, in.Password); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.Action == "delete" && in.Confirm != "DELETE ACCOUNT" {
		writeError(w, 428, errors.New("请确认将账号移入回收站"))
		return
	}
	if in.Action == "password" {
		hash, err := hashPassword(in.Password)
		if err != nil {
			writeError(w, 500, err)
			return
		}
		in.PasswordHash = hash
	}
	item, err := s.repo.AdminMutateUser(r.Context(), r.PathValue("id"), actor.ID, in.AdminUserChange)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	if accountChangeRevokes(in.Action) {
		s.invalidateLiveUser(r.Context(), item.ID)
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: item})
}
func (s *Server) adminBatchUsers(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "user.manage")
	if !ok {
		return
	}
	var in struct {
		Action  string `json:"action"`
		Reason  string `json:"reason"`
		Confirm string `json:"confirm"`
		Targets []struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		} `json:"targets"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if len(in.Targets) == 0 || len(in.Targets) > 100 || !slices.Contains([]string{"ban", "unban", "delete", "restore"}, in.Action) {
		writeError(w, 400, errors.New("请选择 1–100 个账号和有效操作"))
		return
	}
	if in.Action == "delete" && in.Confirm != "DELETE ACCOUNTS" {
		writeError(w, 428, errors.New("请确认批量移入回收站"))
		return
	}
	if err := validateAdminUserChange(AdminUserChange{Version: 1, Action: in.Action, Reason: in.Reason}, ""); err != nil {
		writeError(w, 400, err)
		return
	}
	results := []map[string]any{}
	seen := map[string]bool{}
	for _, target := range in.Targets {
		result := map[string]any{"id": target.ID, "ok": false}
		if seen[target.ID] {
			result["error"] = "重复的账号"
			results = append(results, result)
			continue
		}
		seen[target.ID] = true
		_, err := s.repo.AdminMutateUser(r.Context(), target.ID, actor.ID, AdminUserChange{Version: target.Version, Action: in.Action, Reason: in.Reason})
		if err != nil {
			result["error"] = err.Error()
		} else {
			result["ok"] = true
			s.invalidateLiveUser(r.Context(), target.ID)
		}
		results = append(results, result)
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"results": results}})
}
func (s *Server) invalidateLiveUser(ctx context.Context, id string) {
	s.hub.DisconnectUser(id)
	payload, _ := json.Marshal(map[string]string{"user_id": id})
	s.emitEnvelope(ctx, Envelope{Type: "admin.user.invalidated", From: "server", Payload: payload})
}
func (s *Server) sitePolicy(ctx context.Context) (SitePolicy, AdminDocument, error) {
	return loadSitePolicy(ctx, s.repo)
}
func loadSitePolicy(ctx context.Context, repo Repository) (SitePolicy, AdminDocument, error) {
	d, err := repo.GetAdminDocument(ctx, "site_policy")
	p := defaultSitePolicy()
	if err == nil && len(d.Data) > 0 {
		err = json.Unmarshal(d.Data, &p)
	}
	return p, d, err
}
func validateSitePolicy(p SitePolicy) error {
	if p.MaxRoomsPerUser < 0 || p.MaxRoomsPerUser > 1000 || p.DefaultMaxMembers < 2 || p.DefaultMaxMembers > p.MaxRoomMembers || p.MaxRoomMembers > 100 || p.ChatHistoryLimit < 1 || p.ChatHistoryLimit > 100 {
		return errors.New("房间数需为 0–1000，人数需为 2–100，聊天历史条数需为 1–100")
	}
	return nil
}
func (s *Server) adminPolicy(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "config.write")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		p, d, err := s.sitePolicy(r.Context())
		if err != nil {
			writeAdminError(w, err)
			return
		}
		writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"version": d.Version, "policy": p}})
		return
	}
	var in struct {
		Version int64      `json:"version"`
		Policy  SitePolicy `json:"policy"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if r.Header.Get("X-Confirm-Dangerous") != "update-site-policy" {
		writeError(w, 428, errors.New("请确认更新站点策略"))
		return
	}
	if err := validateSitePolicy(in.Policy); err != nil {
		writeError(w, 400, err)
		return
	}
	data, _ := json.Marshal(in.Policy)
	d, err := s.repo.PutAdminDocument(r.Context(), AdminDocument{Key: "site_policy", Version: in.Version, Data: data}, actor.ID)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	s.emitEnvelope(r.Context(), Envelope{Type: "admin.policy.changed", From: "server"})
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"version": d.Version, "policy": in.Policy}})
}
func (s *Server) taxonomy(ctx context.Context) ([]TaxonomyEntry, AdminDocument, error) {
	d, err := s.repo.GetAdminDocument(ctx, "taxonomy")
	entries := []TaxonomyEntry{}
	if err == nil && len(d.Data) > 0 {
		err = json.Unmarshal(d.Data, &entries)
	}
	return entries, d, err
}
func (s *Server) adminTaxonomy(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "taxonomy.manage")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		entries, d, err := s.taxonomy(r.Context())
		if err != nil {
			writeAdminError(w, err)
			return
		}
		writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"version": d.Version, "entries": entries}})
		return
	}
	var in struct {
		Version int64           `json:"version"`
		Entries []TaxonomyEntry `json:"entries"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if len(in.Entries) > 200 {
		writeError(w, 400, errors.New("分类和标签合计最多 200 项"))
		return
	}
	seen := map[string]bool{}
	categories := map[string]bool{}
	for _, item := range in.Entries {
		if !slices.Contains([]string{"category", "tag"}, item.Kind) || !safeModerationText(item.Key, 30, false) || strings.TrimSpace(item.Key) != item.Key || !safeModerationText(item.Name, 40, false) || !safeModerationText(item.Description, 300, true) || item.SortOrder < 0 || item.SortOrder > 10000 || seen[item.Kind+":"+item.Key] {
			writeError(w, 400, errors.New("分类或标签格式无效，标识不能重复"))
			return
		}
		seen[item.Kind+":"+item.Key] = true
		if item.Kind == "category" {
			categories[item.Key] = true
		}
	}
	for _, item := range in.Entries {
		if item.Category != "" && (item.Kind != "tag" || !categories[item.Category]) {
			writeError(w, 400, errors.New("标签关联的分类不存在"))
			return
		}
	}
	sort.SliceStable(in.Entries, func(i, j int) bool { return in.Entries[i].SortOrder < in.Entries[j].SortOrder })
	data, _ := json.Marshal(in.Entries)
	d, err := s.repo.PutAdminDocument(r.Context(), AdminDocument{Key: "taxonomy", Version: in.Version, Data: data}, actor.ID)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"version": d.Version, "entries": in.Entries}})
}
func (s *Server) publicTaxonomy(w http.ResponseWriter, r *http.Request) {
	entries, _, err := s.taxonomy(r.Context())
	if err != nil {
		writeError(w, 503, errors.New("分类服务暂不可用"))
		return
	}
	enabled := []TaxonomyEntry{}
	categories := map[string]bool{}
	for _, item := range entries {
		if item.Kind == "category" && item.Enabled {
			categories[item.Key] = true
		}
	}
	for _, item := range entries {
		if item.Enabled && (item.Category == "" || categories[item.Category]) {
			enabled = append(enabled, item)
		}
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"entries": enabled}})
}
