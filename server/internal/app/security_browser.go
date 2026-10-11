package app

import (
	"encoding/json"
	"net/http"
	"time"
)

type browserCeremony struct {
	Device      DeviceInfo `json:"device"`
	ResultToken string     `json:"result_token"`
}
type browserResult struct {
	Device  DeviceInfo `json:"device"`
	UserID  string     `json:"user_id"`
	Version int64      `json:"version"`
}

func (s *Server) browserLoginStart(w http.ResponseWriter, r *http.Request) {
	if !s.enforceRateLimit(w, r, "browser-start", sourceIdentity(r), 10, 15*time.Minute) {
		return
	}
	device, e := deviceFromRequest(r)
	if e != nil {
		writeError(w, 400, e)
		return
	}
	result := mustRandomString(32)
	token, e := saveChallenge(r.Context(), s.repo, "browser-login", browserCeremony{Device: device, ResultToken: result})
	if e != nil {
		writeError(w, 500, e)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"url": "/admin/account.html#bridge=" + token, "poll_token": result}})
}
func (s *Server) browserLoginFinish(w http.ResponseWriter, r *http.Request) {
	user, e := s.userFromRequest(r)
	if e != nil {
		writeError(w, 401, e)
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if e = decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	var c browserCeremony
	if e = consumeChallenge(r.Context(), s.repo, in.Token, "browser-login", &c); e != nil {
		writeError(w, 400, e)
		return
	}
	raw, _ := json.Marshal(browserResult{Device: c.Device, UserID: user.ID, Version: user.SessionVersion})
	e = s.repo.SaveAuthChallenge(r.Context(), c.ResultToken, AuthChallenge{Purpose: "browser-result", Data: raw, ExpiresAt: time.Now().Add(2 * time.Minute)})
	if e != nil {
		writeError(w, 500, e)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Msg: "已授权，请返回同帧客户端"})
}
func (s *Server) browserLoginExchange(w http.ResponseWriter, r *http.Request) {
	if !s.enforceRateLimit(w, r, "browser-poll", sourceIdentity(r), 100, time.Minute) {
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	var result browserResult
	e := consumeChallenge(r.Context(), s.repo, in.Token, "browser-result", &result)
	if e != nil {
		writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"pending": true}})
		return
	}
	if result.Device.ID != r.Header.Get("X-Device-ID") {
		writeError(w, 401, ErrUnauthorized)
		return
	}
	a, e := s.repo.UserByID(r.Context(), result.UserID)
	if e != nil || !accountActive(a.User) || a.SessionVersion != result.Version {
		writeError(w, 401, ErrUnauthorized)
		return
	}
	hash := hashDeviceID(result.Device.ID)
	if e = s.repo.BindDevice(r.Context(), a.ID, hash, result.Device); e != nil {
		writeError(w, 403, e)
		return
	}
	session, e := s.auth.issueSession(r.Context(), a.User, hash)
	if e != nil {
		writeError(w, 401, e)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: session})
}
