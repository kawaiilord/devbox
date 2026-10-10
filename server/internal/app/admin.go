package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

var rolePermissions = map[string]map[string]bool{
	"super_admin": {"*": true},
	"admin":       {"report.review": true, "compliance.review": true, "broadcast.send": true, "bot.manage": true, "announcement.manage": true, "order.view": true, "order.reconcile": true, "observability.view": true, "user.view": true, "user.vip.manage": true, "device.ban": true, "device.unban": true, "audit.view": true},
	"operator":    {"broadcast.send": true, "announcement.manage": true, "observability.view": true, "user.view": true},
	"seller":      {"order.view": true, "activation_code.manage": true},
}

func (s *Server) requirePermission(w http.ResponseWriter, r *http.Request, permission string) (User, bool) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return User{}, false
	}
	account, err := s.repo.UserByID(r.Context(), user.ID)
	if err != nil || !account.EmailVerified {
		writeError(w, http.StatusForbidden, ErrForbidden)
		return User{}, false
	}
	role := account.AdminRole
	if role == "" && account.IsAdmin {
		role = "super_admin"
	}
	allowed := rolePermissions[role]
	if !allowed["*"] && !allowed[permission] {
		writeError(w, http.StatusForbidden, ErrForbidden)
		return User{}, false
	}
	return account.User, true
}

func (s *Server) withMaintenance(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/healthz" || path == "/api/v1/config" || path == "/api/v1/announcements" || strings.HasPrefix(path, "/api/v1/admin/") || strings.HasPrefix(path, "/api/v1/auth/") || path == "/api/v1/payments/callback" || strings.HasPrefix(path, "/admin") {
			next.ServeHTTP(w, r)
			return
		}
		config, err := s.repo.GetRuntimeConfig(r.Context())
		if err == nil && config.Maintenance.Enabled {
			w.Header().Set("Retry-After", "300")
			message := config.Maintenance.Message
			if message == "" {
				message = "service maintenance"
			}
			writeError(w, http.StatusServiceUnavailable, errors.New(message))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) publicAnnouncements(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListAnnouncements(r.Context(), true, 0, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"announcements": items}, Msg: "ok"})
}

func (s *Server) adminGetConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "config.read"); !ok {
		return
	}
	config, err := s.repo.GetRuntimeConfig(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: config, Msg: "ok"})
}

func (s *Server) adminUpdateConfig(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, "config.write")
	if !ok {
		return
	}
	if r.Header.Get("X-Confirm-Dangerous") != "update-runtime-config" {
		writeError(w, http.StatusPreconditionRequired, errors.New("explicit configuration confirmation required"))
		return
	}
	var config RuntimeConfig
	if err := decodeJSON(r, &config); err != nil {
		writeError(w, 400, err)
		return
	}
	if !safeModerationText(config.Maintenance.Message, 500, true) || !safeModerationText(config.Branding.GlobalAnnouncement, 1000, true) || len(config.Features) > 100 {
		writeError(w, 400, errors.New("invalid runtime configuration"))
		return
	}
	for key := range config.Features {
		if !safeFeatureKey(key) {
			writeError(w, 400, errors.New("invalid feature key"))
			return
		}
	}
	config, err := s.repo.UpdateRuntimeConfig(r.Context(), config, admin.ID)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: config, Msg: "updated"})
}

func (s *Server) adminAnnouncements(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "announcement.manage"); !ok {
		return
	}
	before, limit := moderationPage(r)
	items, err := s.repo.ListAnnouncements(r.Context(), false, before, limit)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"announcements": items}, Msg: "ok"})
}

func (s *Server) adminCreateAnnouncement(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, "announcement.manage")
	if !ok {
		return
	}
	var item Announcement
	if err := decodeJSON(r, &item); err != nil {
		writeError(w, 400, err)
		return
	}
	item.Title = strings.TrimSpace(item.Title)
	item.Body = strings.TrimSpace(item.Body)
	item.CreatedBy = admin.ID
	if !safeModerationText(item.Title, 160, false) || !safeModerationText(item.Body, 5000, false) || !slices.Contains([]string{"list", "startup", "room"}, item.Kind) || (item.EndsAt != 0 && item.StartsAt != 0 && item.EndsAt <= item.StartsAt) {
		writeError(w, 400, errors.New("invalid announcement"))
		return
	}
	created, err := s.repo.CreateAnnouncement(r.Context(), item)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if created.Kind == "room" && created.Active {
		s.broadcastAnnouncement(created)
	}
	writeJSON(w, 201, apiResponse{Code: 0, Data: created, Msg: "created"})
}

func (s *Server) adminDeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, "announcement.manage")
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if err = s.repo.DeleteAnnouncement(r.Context(), id, admin.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Msg: "deleted"})
}

func (s *Server) adminBroadcast(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, "broadcast.send")
	if !ok {
		return
	}
	var request struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	if !safeModerationText(request.Title, 160, false) || !safeModerationText(request.Body, 2000, false) {
		writeError(w, 400, errors.New("invalid broadcast"))
		return
	}
	item := Announcement{Title: request.Title, Body: request.Body, Kind: "room", Active: true, CreatedBy: admin.ID, CreatedAt: time.Now().UnixMilli()}
	s.broadcastAnnouncement(item)
	_, _ = s.repo.AppendAudit(r.Context(), AuditEvent{ActorID: admin.ID, Action: "broadcast.send", TargetType: "rooms", TargetID: "all"})
	writeJSON(w, 200, apiResponse{Code: 0, Msg: "broadcast"})
}

func (s *Server) broadcastAnnouncement(item Announcement) {
	raw, _ := json.Marshal(item)
	s.hub.BroadcastAll(Envelope{Type: "announcement", TS: time.Now().UnixMilli(), Payload: raw})
}

func (s *Server) adminDeviceBans(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "device.ban"); !ok {
		return
	}
	before, limit := moderationPage(r)
	items, err := s.repo.ListDeviceBans(r.Context(), before, limit)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"bans": items}, Msg: "ok"})
}
func (s *Server) adminUnbanDevice(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, "device.unban")
	if !ok {
		return
	}
	hash := strings.ToLower(strings.TrimSpace(r.PathValue("hash")))
	if !deviceHashPattern.MatchString(hash) {
		writeError(w, 400, errors.New("invalid device hash"))
		return
	}
	if err := s.repo.UnbanDevice(r.Context(), hash, admin.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Msg: "unbanned"})
}

func (s *Server) adminDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "observability.view"); !ok {
		return
	}
	value, err := s.repo.AdminDashboard(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: value, Msg: "ok"})
}
func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "user.view"); !ok {
		return
	}
	before, limit := moderationPage(r)
	items, err := s.repo.SearchAdminUsers(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), before, limit)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"users": items}, Msg: "ok"})
}
func (s *Server) adminSetRole(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, "role.manage")
	if !ok {
		return
	}
	var request struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	if !slices.Contains([]string{"", "admin", "operator", "seller", "super_admin"}, request.Role) || r.PathValue("id") == admin.ID {
		writeError(w, 400, errors.New("invalid role update"))
		return
	}
	if err := s.repo.SetAdminRole(r.Context(), r.PathValue("id"), request.Role, admin.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Msg: "updated"})
}

func (s *Server) adminOrders(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "order.view"); !ok {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !slices.Contains([]string{"pending", "paid", "activated", "expired", "cancelled"}, status) {
		writeError(w, 400, errors.New("invalid order status"))
		return
	}
	before, limit := moderationPage(r)
	items, err := s.repo.AdminListOrders(r.Context(), status, before, limit)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"orders": items}, Msg: "ok"})
}
func (s *Server) adminPlans(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "order.view"); !ok {
		return
	}
	items, err := s.repo.ListVIPPlans(r.Context(), true)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"plans": items}, Msg: "ok"})
}
func (s *Server) adminUpdatePlan(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, "plan.manage")
	if !ok {
		return
	}
	var plan VIPPlan
	if err := decodeJSON(r, &plan); err != nil {
		writeError(w, 400, err)
		return
	}
	plan.ID = r.PathValue("id")
	if !safeModerationText(plan.Title, 100, false) || plan.PriceMinor < 0 || plan.OriginalPriceMinor < 0 || plan.DurationDays < 0 || (!plan.Lifetime && plan.DurationDays < 1) {
		writeError(w, 400, errors.New("invalid plan"))
		return
	}
	saved, err := s.repo.UpdateVIPPlan(r.Context(), plan, admin.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: saved, Msg: "updated"})
}

func (s *Server) adminGetRoomBot(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "bot.manage"); !ok {
		return
	}
	config, _, err := s.repo.GetRoomBotConfig(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: config, Msg: "ok"})
}
func (s *Server) adminUpdateRoomBot(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, "bot.manage")
	if !ok {
		return
	}
	var config RoomBotConfig
	if err := decodeJSON(r, &config); err != nil {
		writeError(w, 400, err)
		return
	}
	if !safeModerationText(config.DisplayName, 80, false) || !slices.Contains([]string{"admin", "vip", "allowlist", "all"}, config.SummonPolicy) || !slices.Contains([]string{"mention", "all", "off"}, config.ReplyPolicy) {
		writeError(w, 400, errors.New("invalid bot config"))
		return
	}
	if config.ProviderBaseURL != "" {
		parsed, err := url.Parse(config.ProviderBaseURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			writeError(w, 400, errors.New("invalid bot provider URL"))
			return
		}
	}
	cipher := ""
	if strings.TrimSpace(config.Credential) != "" {
		if s.options.Vault == nil {
			writeError(w, 503, errors.New("credential vault unavailable"))
			return
		}
		var err error
		cipher, err = s.options.Vault.Encrypt([]byte(config.Credential), "room-bot:singleton")
		if err != nil {
			writeError(w, 500, err)
			return
		}
	}
	config.Credential = ""
	saved, err := s.repo.UpdateRoomBotConfig(r.Context(), config, cipher, admin.ID)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: saved, Msg: "updated"})
}

func safeFeatureKey(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}
