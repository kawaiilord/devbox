package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type RTCConfigManager struct {
	turnURLs []string
	secret   []byte
	ttl      time.Duration
}

type RTCServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type RTCSignal struct {
	Type          string `json:"type"`
	TargetUserID  string `json:"target_user_id"`
	SDP           string `json:"sdp,omitempty"`
	Candidate     string `json:"candidate,omitempty"`
	SDPMid        string `json:"sdp_mid,omitempty"`
	SDPMLineIndex int    `json:"sdp_mline_index"`
}

func NewRTCConfigManager(turnURLs []string, sharedSecret string) (*RTCConfigManager, error) {
	clean := make([]string, 0, len(turnURLs))
	for _, raw := range turnURLs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.HasPrefix(raw, "turn:") && !strings.HasPrefix(raw, "turns:") {
			return nil, errors.New("TURN URLs must use turn: or turns:")
		}
		clean = append(clean, raw)
	}
	if len(clean) == 0 || len(sharedSecret) < 16 {
		return nil, errors.New("TURN URLs and a shared secret are required")
	}
	return &RTCConfigManager{turnURLs: clean, secret: []byte(sharedSecret), ttl: time.Hour}, nil
}

func (m *RTCConfigManager) Servers(userID string, now time.Time) ([]RTCServer, int64) {
	expires := now.Add(m.ttl).Unix()
	username := strings.Join([]string{strconv.FormatInt(expires, 10), userID}, ":")
	digest := hmac.New(sha1.New, m.secret)
	_, _ = digest.Write([]byte(username))
	credential := base64.StdEncoding.EncodeToString(digest.Sum(nil))
	return []RTCServer{{URLs: append([]string(nil), m.turnURLs...), Username: username, Credential: credential}}, expires * 1000
}

func (s *Server) rtcConfig(w http.ResponseWriter, r *http.Request) {
	if s.options.RTC == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("voice unavailable"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	if _, err := s.store.GetRoom(code, user.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	servers, expires := s.options.RTC.Servers(user.ID, time.Now())
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"ice_servers": servers, "expires_at": expires}, Msg: "ok"})
}

func (s *Server) handleRTCSignal(ctx context.Context, client *socketClient, roomCode string, user User, payload json.RawMessage) {
	room, err := s.store.GetRoom(roomCode, user.ID)
	if err != nil {
		s.sendSocketError(client, roomCode, ErrForbidden)
		return
	}
	var signal RTCSignal
	if json.Unmarshal(payload, &signal) != nil {
		s.sendSocketError(client, roomCode, errors.New("invalid RTC signal"))
		return
	}
	signal.Type = strings.ToLower(strings.TrimSpace(signal.Type))
	signal.TargetUserID = strings.TrimSpace(signal.TargetUserID)
	if signal.TargetUserID == "" || signal.TargetUserID == user.ID || !rtcSignalType(signal.Type) || len(signal.SDP) > 64*1024 || len(signal.Candidate) > 4096 || len(signal.SDPMid) > 256 || signal.SDPMLineIndex < 0 || !utf8.ValidString(signal.SDP) || !utf8.ValidString(signal.Candidate) {
		s.sendSocketError(client, roomCode, errors.New("invalid RTC signal"))
		return
	}
	found := false
	for _, member := range room.Members {
		if member.UserID == signal.TargetUserID {
			found = true
			break
		}
	}
	if !found {
		s.sendSocketError(client, roomCode, ErrNotFound)
		return
	}
	encoded, _ := json.Marshal(signal)
	s.emitEnvelope(ctx, Envelope{Type: "rtc.signal", Room: roomCode, TS: time.Now().UnixMilli(), From: user.ID, Payload: encoded})
}
func rtcSignalType(value string) bool {
	switch value {
	case "ready", "offer", "answer", "candidate", "hangup", "mute":
		return true
	default:
		return false
	}
}
