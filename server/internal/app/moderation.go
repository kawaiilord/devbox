package app

import (
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var deviceHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (s *Server) getPrivacy(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	settings, err := s.repo.GetPrivacy(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load privacy settings"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: settings, Msg: "ok"})
}

func (s *Server) updatePrivacy(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		AllowRoomChat     *bool `json:"allow_room_chat"`
		AllowProfileFind  *bool `json:"allow_profile_find"`
		ShowWatchActivity *bool `json:"show_watch_activity"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, err := s.repo.GetPrivacy(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load privacy settings"))
		return
	}
	if request.AllowRoomChat != nil {
		settings.AllowRoomChat = *request.AllowRoomChat
	}
	if request.AllowProfileFind != nil {
		settings.AllowProfileFind = *request.AllowProfileFind
	}
	if request.ShowWatchActivity != nil {
		settings.ShowWatchActivity = *request.ShowWatchActivity
	}
	if err := s.repo.UpdatePrivacy(r.Context(), user.ID, settings); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not update privacy settings"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: settings, Msg: "updated"})
}

func (s *Server) listBlocks(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	users, err := s.repo.ListBlockedUsers(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load blocked users"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{
		Code: 0, Data: map[string]any{"users": users}, Msg: "ok",
	})
}

func (s *Server) blockUser(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	blockedID := strings.TrimSpace(r.PathValue("user_id"))
	if blockedID == "" || len(blockedID) > 128 {
		writeError(w, http.StatusBadRequest, errors.New("invalid user identifier"))
		return
	}
	if err := s.repo.BlockUser(r.Context(), user.ID, blockedID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "blocked"})
}

func (s *Server) unblockUser(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	blockedID := strings.TrimSpace(r.PathValue("user_id"))
	if blockedID == "" || len(blockedID) > 128 {
		writeError(w, http.StatusBadRequest, errors.New("invalid user identifier"))
		return
	}
	if err := s.repo.UnblockUser(r.Context(), user.ID, blockedID); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not unblock user"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "unblocked"})
}

func (s *Server) createReport(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if !s.enforceRateLimit(w, r, "report", user.ID, 10, time.Hour) {
		return
	}
	var request struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		Reason     string `json:"reason"`
		Details    string `json:"details"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.TargetType = strings.ToLower(strings.TrimSpace(request.TargetType))
	request.TargetID = strings.TrimSpace(request.TargetID)
	request.Reason = strings.ToLower(strings.TrimSpace(request.Reason))
	request.Details = strings.TrimSpace(request.Details)
	if !slices.Contains([]string{"room", "message", "user"}, request.TargetType) ||
		!safeModerationIdentifier(request.TargetID, 256) ||
		!slices.Contains([]string{"spam", "harassment", "illegal", "copyright", "other"}, request.Reason) ||
		!safeModerationText(request.Details, 1000, true) {
		writeError(w, http.StatusBadRequest, errors.New("invalid report"))
		return
	}
	report, err := s.repo.CreateReport(r.Context(), Report{
		ReporterID: user.ID, TargetType: request.TargetType, TargetID: request.TargetID,
		Reason: request.Reason, Details: request.Details,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not create report"))
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: report, Msg: "created"})
}

func (s *Server) adminReports(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if status != "" && !slices.Contains([]string{"pending", "actioned", "dismissed"}, status) {
		writeError(w, http.StatusBadRequest, errors.New("invalid report status"))
		return
	}
	before, limit := moderationPage(r)
	reports, err := s.repo.ListReports(r.Context(), status, before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load reports"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{
		Code: 0, Data: map[string]any{"reports": reports}, Msg: "ok",
	})
}

func (s *Server) resolveReport(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, errors.New("invalid report identifier"))
		return
	}
	var request struct {
		Status     string `json:"status"`
		Resolution string `json:"resolution"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.Status = strings.ToLower(strings.TrimSpace(request.Status))
	request.Resolution = strings.TrimSpace(request.Resolution)
	if !slices.Contains([]string{"actioned", "dismissed"}, request.Status) ||
		!safeModerationText(request.Resolution, 1000, true) {
		writeError(w, http.StatusBadRequest, errors.New("invalid report resolution"))
		return
	}
	report, err := s.repo.ResolveReport(r.Context(), id, admin.ID, request.Status, request.Resolution)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: report, Msg: "resolved"})
}

func (s *Server) adminCloseRoom(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.PathValue("code")))
	if len(code) != 6 {
		writeError(w, http.StatusBadRequest, errors.New("invalid room code"))
		return
	}
	if err := s.repo.CloseRoom(r.Context(), code, admin.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	room, sequence, err := s.store.CloseRoom(code)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var realtimeErr error
	if s.redis != nil {
		if err := s.redis.CacheRoom(r.Context(), room); err != nil {
			realtimeErr = err
		} else {
			sequence, realtimeErr = s.redis.NextSequence(r.Context(), code)
		}
	}
	s.broadcastRoomState(r.Context(), room, sequence)
	if realtimeErr != nil {
		s.options.Logger.Error("broadcast closed room", "error", realtimeErr, "room", code)
		writeError(w, http.StatusServiceUnavailable, errors.New("room closed but realtime propagation is degraded"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: room, Msg: "closed"})
}

func (s *Server) adminBanDevice(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	deviceHash := strings.ToLower(strings.TrimSpace(r.PathValue("hash")))
	if !deviceHashPattern.MatchString(deviceHash) {
		writeError(w, http.StatusBadRequest, errors.New("invalid device hash"))
		return
	}
	var request struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if !safeModerationText(request.Reason, 256, false) {
		writeError(w, http.StatusBadRequest, errors.New("invalid ban reason"))
		return
	}
	if err := s.repo.BanDevice(r.Context(), deviceHash, request.Reason, admin.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "banned"})
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	before, limit := moderationPage(r)
	events, err := s.repo.ListAudit(r.Context(), before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load audit log"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{
		"events": events, "chain_valid": verifyAuditPage(events),
	}, Msg: "ok"})
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (User, bool) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return User{}, false
	}
	account, err := s.repo.UserByID(r.Context(), user.ID)
	if err != nil || !account.IsAdmin || !account.EmailVerified {
		writeError(w, http.StatusForbidden, ErrForbidden)
		return User{}, false
	}
	return account.User, true
}

func moderationPage(r *http.Request) (int64, int) {
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	return before, limit
}

func safeModerationText(value string, limit int, allowEmpty bool) bool {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return false
	}
	length := utf8.RuneCountInString(value)
	if (!allowEmpty && length == 0) || length > limit {
		return false
	}
	return !strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\t'
	})
}

func safeModerationIdentifier(value string, limit int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) > 0 &&
		utf8.RuneCountInString(value) <= limit &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func verifyAuditPage(events []AuditEvent) bool {
	for index, event := range events {
		expected := finalizeAuditEvent(AuditEvent{
			ActorID: event.ActorID, Action: event.Action, TargetType: event.TargetType,
			TargetID: event.TargetID, Metadata: event.Metadata,
			PreviousHash: event.PreviousHash, CreatedAt: event.CreatedAt,
		})
		if expected.EntryHash != event.EntryHash {
			return false
		}
		if index+1 < len(events) && event.PreviousHash != events[index+1].EntryHash {
			return false
		}
	}
	return true
}
