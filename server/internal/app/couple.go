package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) coupleInfo(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	couple, err := s.repo.GetCouple(r.Context(), user.ID)
	if errors.Is(err, ErrNotFound) {
		writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"has_couple": false}, Msg: "ok"})
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"has_couple": true, "couple": couple}, Msg: "ok"})
}
func (s *Server) requestCouple(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	account, err := s.repo.UserByID(r.Context(), user.ID)
	if err != nil || account.VIPExpiresAt <= time.Now().UnixMilli() {
		writeError(w, http.StatusForbidden, errors.New("active membership required"))
		return
	}
	var request struct {
		UserID string `json:"user_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := s.repo.CreateCoupleRequest(r.Context(), user.ID, strings.TrimSpace(request.UserID))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: item, Msg: "requested"})
}
func (s *Server) coupleRequests(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	items, err := s.repo.ListCoupleRequests(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"requests": items}, Msg: "ok"})
}
func (s *Server) respondCouple(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var request struct {
		Accept bool `json:"accept"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	couple, err := s.repo.RespondCoupleRequest(r.Context(), user.ID, id, request.Accept)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: couple, Msg: "reviewed"})
}
func (s *Server) separateCouple(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	couple, err := s.repo.SeparateCouple(r.Context(), user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: couple, Msg: "separated"})
}
func (s *Server) restoreCouple(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	couple, err := s.repo.RestoreCouple(r.Context(), user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: couple, Msg: "restored"})
}
func (s *Server) addCoupleMoment(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.Body = strings.TrimSpace(request.Body)
	if !safeModerationText(request.Body, 1000, false) {
		writeError(w, http.StatusBadRequest, errors.New("invalid moment"))
		return
	}
	moment, err := s.repo.AddCoupleMoment(r.Context(), user.ID, request.Body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: moment, Msg: "created"})
}
func (s *Server) coupleMoments(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	items, err := s.repo.ListCoupleMoments(r.Context(), user.ID, before, 50)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"moments": items}, Msg: "ok"})
}
func (s *Server) coupleTimeline(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	items, err := s.repo.ListCoupleEvents(r.Context(), user.ID, 100)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"events": items}, Msg: "ok"})
}
func (s *Server) coupleSharedFavorites(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	couple, err := s.repo.GetCouple(r.Context(), user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	mine, _ := s.repo.ListFavorites(r.Context(), user.ID, 0, 100)
	theirs, _ := s.repo.ListFavorites(r.Context(), couple.Partner.ID, 0, 100)
	names := map[string]bool{}
	for _, f := range theirs {
		names[strings.ToLower(f.Title)] = true
	}
	shared := []Favorite{}
	for _, f := range mine {
		if names[strings.ToLower(f.Title)] {
			shared = append(shared, f)
		}
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"favorites": shared}, Msg: "ok"})
}
func (s *Server) coupleSharedMovies(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	couple, err := s.repo.GetCouple(r.Context(), user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	mine, _ := s.repo.ListWatchRecords(r.Context(), user.ID, 0, 100)
	theirs, _ := s.repo.ListWatchRecords(r.Context(), couple.Partner.ID, 0, 100)
	names := map[string]bool{}
	for _, v := range theirs {
		names[strings.ToLower(v.Title)] = true
	}
	shared := []WatchActivity{}
	for _, v := range mine {
		if names[strings.ToLower(v.Title)] {
			shared = append(shared, WatchActivity{Title: v.Title, Episode: v.Episode, Completed: v.Completed, CompanionCount: v.CompanionCount, WatchedAt: v.WatchedAt})
		}
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"movies": shared}, Msg: "ok"})
}
