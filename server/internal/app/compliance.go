package app

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (s *Server) requestAccountDeletion(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	var request struct {
		Reason  string `json:"reason"`
		Confirm string `json:"confirm"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Confirm != "DELETE MY ACCOUNT" || !safeModerationText(request.Reason, 1000, false) {
		writeError(w, 400, errors.New("invalid deletion request or confirmation"))
		return
	}
	item, err := s.repo.CreateAccountDeletionRequest(r.Context(), user.ID, request.Reason)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 201, apiResponse{Code: 0, Data: item, Msg: "requested"})
}
func (s *Server) getAccountDeletion(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	item, err := s.repo.GetAccountDeletionRequest(r.Context(), user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: item, Msg: "ok"})
}
func (s *Server) cancelAccountDeletion(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	if err := s.repo.CancelAccountDeletionRequest(r.Context(), user.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Msg: "cancelled"})
}

func (s *Server) createCopyrightComplaint(w http.ResponseWriter, r *http.Request) {
	if !s.enforceRateLimit(w, r, "copyright-complaint", sourceIdentity(r), 5, 24*time.Hour) {
		return
	}
	var item CopyrightComplaint
	if err := decodeJSON(r, &item); err != nil {
		writeError(w, 400, err)
		return
	}
	if user, err := s.userFromRequest(r); err == nil {
		item.ClaimantUserID = user.ID
	}
	item.ClaimantName = strings.TrimSpace(item.ClaimantName)
	item.ClaimantEmail = strings.ToLower(strings.TrimSpace(item.ClaimantEmail))
	item.RightsBasis = strings.TrimSpace(item.RightsBasis)
	item.InfringementURL = strings.TrimSpace(item.InfringementURL)
	item.RoomCode = strings.ToUpper(strings.TrimSpace(item.RoomCode))
	item.SignatureName = strings.TrimSpace(item.SignatureName)
	parsed, parseErr := url.Parse(item.InfringementURL)
	if !safeModerationText(item.ClaimantName, 160, false) || !validEmail(item.ClaimantEmail) || !safeModerationText(item.RightsBasis, 3000, false) || parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || (item.RoomCode != "" && len(item.RoomCode) != 6) || len(item.Evidence) > 20 || !item.StatementAccurate || !safeModerationText(item.SignatureName, 160, false) {
		writeError(w, 400, errors.New("invalid copyright complaint"))
		return
	}
	for _, evidence := range item.Evidence {
		parsed, err := url.Parse(evidence)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || len(evidence) > 2048 {
			writeError(w, 400, errors.New("invalid complaint evidence URL"))
			return
		}
	}
	created, err := s.repo.CreateCopyrightComplaint(r.Context(), item)
	if err != nil {
		writeError(w, 500, errors.New("could not submit complaint"))
		return
	}
	writeJSON(w, 201, apiResponse{Code: 0, Data: map[string]any{"id": created.ID, "status": created.Status, "due_at": created.DueAt}, Msg: "submitted"})
}

func (s *Server) adminAccountDeletions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "compliance.review"); !ok {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !slices.Contains([]string{"pending", "approved", "rejected", "cancelled"}, status) {
		writeError(w, 400, errors.New("invalid status"))
		return
	}
	before, limit := moderationPage(r)
	items, err := s.repo.ListAccountDeletionRequests(r.Context(), status, before, limit)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"requests": items}, Msg: "ok"})
}
func (s *Server) adminResolveAccountDeletion(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, "compliance.review")
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var request struct {
		Approve    bool   `json:"approve"`
		Resolution string `json:"resolution"`
		Confirm    string `json:"confirm"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	request.Resolution = strings.TrimSpace(request.Resolution)
	if !safeModerationText(request.Resolution, 2000, false) || (request.Approve && request.Confirm != "IRREVERSIBLY DELETE ACCOUNT") {
		writeError(w, 400, errors.New("invalid resolution or confirmation"))
		return
	}
	item, err := s.repo.ResolveAccountDeletionRequest(r.Context(), id, admin.ID, request.Approve, request.Resolution)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if request.Approve && item.UserID != "" {
		for _, room := range s.store.CloseRoomsByOwner(item.UserID) {
			sequence := int64(0)
			if s.redis != nil {
				_ = s.redis.CacheRoom(r.Context(), room)
				sequence, _ = s.redis.NextSequence(r.Context(), room.Code)
			}
			s.broadcastRoomState(r.Context(), room, sequence)
		}
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: item, Msg: "resolved"})
}
func (s *Server) adminCopyrightComplaints(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "compliance.review"); !ok {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !slices.Contains([]string{"submitted", "triaged", "actioned", "rejected", "withdrawn"}, status) {
		writeError(w, 400, errors.New("invalid status"))
		return
	}
	before, limit := moderationPage(r)
	items, err := s.repo.ListCopyrightComplaints(r.Context(), status, before, limit)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"complaints": items}, Msg: "ok"})
}
func (s *Server) adminResolveCopyrightComplaint(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, "compliance.review")
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var request struct {
		Status     string `json:"status"`
		Resolution string `json:"resolution"`
		CloseRoom  bool   `json:"close_room"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	request.Resolution = strings.TrimSpace(request.Resolution)
	if !slices.Contains([]string{"triaged", "actioned", "rejected"}, request.Status) || !safeModerationText(request.Resolution, 2000, false) {
		writeError(w, 400, errors.New("invalid complaint resolution"))
		return
	}
	item, err := s.repo.ResolveCopyrightComplaint(r.Context(), id, admin.ID, request.Status, request.Resolution)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if request.CloseRoom && item.RoomCode != "" {
		if err := s.repo.CloseRoom(r.Context(), item.RoomCode, admin.ID); err == nil {
			if room, seq, closeErr := s.store.CloseRoom(item.RoomCode); closeErr == nil {
				s.broadcastRoomState(r.Context(), room, seq)
			}
		}
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: item, Msg: "resolved"})
}

func validEmail(value string) bool {
	at := strings.LastIndex(value, "@")
	return at > 0 && at < len(value)-3 && len(value) <= 320 && strings.Contains(value[at+1:], ".")
}
