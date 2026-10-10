package app

import (
	"errors"
	"net/http"
	"time"
)

func (s *Server) saveQuarkSource(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("media sources unavailable"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	account, err := s.repo.UserByID(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	user = account.User
	if s.options.RequireVerifiedEmail && !user.EmailVerified {
		writeError(w, http.StatusForbidden, errors.New("email verification required"))
		return
	}
	if !s.enforceRateLimit(w, r, "source-create", user.ID, 10, time.Hour) {
		return
	}
	var request struct {
		Name   string `json:"name"`
		Cookie string `json:"cookie"`
	}
	if decodeJSON(r, &request) != nil {
		// Never echo JSON parser diagnostics for a body containing a login cookie.
		writeError(w, http.StatusBadRequest, errors.New("夸克登录信息格式无效"))
		return
	}
	source, err := s.sources.SaveQuark(r.Context(), user, r.PathValue("id"), request.Name, request.Cookie)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrForbidden) {
			writeStoreError(w, err)
		} else {
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}
	status := http.StatusCreated
	if r.Method == http.MethodPut {
		status = http.StatusOK
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, apiResponse{Code: 0, Data: source, Msg: "ok"})
}
