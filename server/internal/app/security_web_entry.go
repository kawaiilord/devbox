package app

import (
	"encoding/json"
	"net/http"
	"time"
)

const webEntryCookie = "__Secure-sameframe-entry"
const webEntryPath = "/api/v1/auth/web-session"

type webEntryProof struct {
	UserID     string `json:"user_id"`
	DeviceHash string `json:"device_hash"`
	Version    int64  `json:"version"`
}

func entryCookie(value string, age int) *http.Cookie {
	return &http.Cookie{Name: webEntryCookie, Value: value, Path: webEntryPath,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: age,
		Expires: time.Now().Add(time.Duration(age) * time.Second)}
}

// The lightweight account form hands off an authenticated identity, never a
// bearer token in the URL or persistent JavaScript-readable storage.
func (s *Server) webEntryStart(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil || !hasRegisteredAccount(user) {
		writeError(w, 401, ErrUnauthorized)
		return
	}
	if !s.enforceRateLimit(w, r, "web-entry-start", user.ID, 15, time.Minute) {
		return
	}
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err = decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if old, e := r.Cookie(webEntryCookie); e == nil {
		_, _ = s.repo.ConsumeAuthChallenge(r.Context(), old.Value, "web-entry")
	}
	token := mustRandomString(32)
	raw, _ := json.Marshal(webEntryProof{UserID: user.ID, DeviceHash: hashDeviceID(r.Header.Get("X-Device-ID")), Version: user.SessionVersion})
	if err = s.repo.SaveAuthChallenge(r.Context(), token, AuthChallenge{Purpose: "web-entry", Data: raw, ExpiresAt: time.Now().Add(2 * time.Minute)}); err != nil {
		writeError(w, 503, err)
		return
	}
	if err = s.auth.Logout(r.Context(), in.RefreshToken); err != nil {
		_, _ = s.repo.ConsumeAuthChallenge(r.Context(), token, "web-entry")
		writeError(w, 503, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, entryCookie(token, 120))
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"ready": true}})
}

func (s *Server) webEntryExchange(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	cookie, err := r.Cookie(webEntryCookie)
	if err != nil {
		writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{}})
		return
	}
	http.SetCookie(w, entryCookie("", -1))
	if !s.enforceRateLimit(w, r, "web-entry-exchange", sourceIdentity(r), 30, time.Minute) {
		return
	}
	proof, err := s.repo.ConsumeAuthChallenge(r.Context(), cookie.Value, "web-entry")
	if err != nil {
		writeError(w, 401, ErrUnauthorized)
		return
	}
	var entry webEntryProof
	if json.Unmarshal(proof.Data, &entry) != nil || entry.DeviceHash != hashDeviceID(r.Header.Get("X-Device-ID")) {
		writeError(w, 401, ErrUnauthorized)
		return
	}
	account, err := s.repo.UserByID(r.Context(), entry.UserID)
	if err != nil || !accountActive(account.User) || !hasRegisteredAccount(account.User) || account.SessionVersion != entry.Version {
		writeError(w, 401, ErrUnauthorized)
		return
	}
	if err = s.repo.ValidateDevice(r.Context(), entry.UserID, entry.DeviceHash); err != nil {
		writeError(w, 401, ErrUnauthorized)
		return
	}
	session, err := s.auth.issueSession(r.Context(), account.User, entry.DeviceHash)
	if err != nil {
		writeError(w, 503, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: session})
}
