package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) roomDanmaku(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.PathValue("code")))
	if err := s.refreshRedisRoom(r.Context(), code); err != nil && !errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusServiceUnavailable, errors.New("realtime state unavailable"))
		return
	}
	room, err := s.store.GetRoom(code, user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	privacy, err := s.repo.GetPrivacy(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load privacy settings"))
		return
	}
	if !privacy.AllowRoomChat {
		writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"messages": []DanmakuMessage{}}, Msg: "ok"})
		return
	}
	episode := room.Playback.Episode
	if value := r.URL.Query().Get("episode"); value != "" {
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil || parsed < 0 || parsed > 100000 {
			writeError(w, http.StatusBadRequest, errors.New("invalid episode"))
			return
		}
		episode = parsed
	}
	from := 0.0
	if value := r.URL.Query().Get("from"); value != "" {
		from, err = strconv.ParseFloat(value, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid danmaku range"))
			return
		}
	}
	to := float64(maxWatchSeconds)
	if value := r.URL.Query().Get("to"); value != "" {
		to, err = strconv.ParseFloat(value, 64)
	}
	if err != nil || !validWatchSeconds(from) || !validWatchSeconds(to) || from > to {
		writeError(w, http.StatusBadRequest, errors.New("invalid danmaku range"))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 2000 {
		limit = 1000
	}
	messages, err := s.repo.ListDanmaku(
		r.Context(), mediaFingerprint(room, episode), user.ID, from, to, limit,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load danmaku"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{
		Code: 0, Data: map[string]any{"messages": messages}, Msg: "ok",
	})
}

func (s *Server) handleDanmakuMessage(
	ctx context.Context,
	client *socketClient,
	roomCode string,
	user User,
	payload json.RawMessage,
) {
	room, err := s.store.GetRoom(roomCode, user.ID)
	if err != nil {
		s.sendSocketError(client, roomCode, ErrForbidden)
		return
	}
	privacy, err := s.repo.GetPrivacy(ctx, user.ID)
	if err != nil || !privacy.AllowRoomChat {
		s.sendSocketError(client, roomCode, ErrForbidden)
		return
	}
	var incoming struct {
		Body     string  `json:"body"`
		Position float64 `json:"position_seconds"`
		Color    int     `json:"color"`
		Mode     string  `json:"mode"`
	}
	if json.Unmarshal(payload, &incoming) != nil {
		s.sendSocketError(client, roomCode, errors.New("invalid danmaku message"))
		return
	}
	incoming.Body = strings.TrimSpace(incoming.Body)
	incoming.Mode = strings.ToLower(strings.TrimSpace(incoming.Mode))
	if !validLibraryText(incoming.Body, 100) || !validWatchSeconds(incoming.Position) ||
		incoming.Color < 0 || incoming.Color > 0xffffff ||
		(incoming.Mode != "scroll" && incoming.Mode != "top" && incoming.Mode != "bottom") {
		s.sendSocketError(client, roomCode, errors.New("invalid danmaku message"))
		return
	}
	room = s.syncRedisPlayback(ctx, room)
	if math.Abs(incoming.Position-room.Playback.Position) > 30 {
		s.sendSocketError(client, roomCode, errors.New("danmaku position is outside the current playback window"))
		return
	}
	if !s.danmakuRateAllowed(ctx, roomCode, user.ID) {
		s.sendSocketError(client, roomCode, errors.New("danmaku rate limit exceeded"))
		return
	}
	message, err := s.repo.AddDanmaku(ctx, DanmakuMessage{
		Fingerprint: mediaFingerprint(room, room.Playback.Episode),
		UserID:      user.ID, DisplayName: user.DisplayName, Body: incoming.Body,
		Position: incoming.Position, Color: incoming.Color, Mode: incoming.Mode,
	})
	if err != nil {
		s.sendSocketError(client, roomCode, errors.New("danmaku persistence failed"))
		return
	}
	encoded, _ := json.Marshal(message)
	s.emitEnvelope(ctx, Envelope{
		Type: "danmaku.message", Room: roomCode, Seq: message.ID,
		TS: message.CreatedAt, From: user.ID, Payload: encoded,
	})
}

func (s *Server) danmakuRateAllowed(ctx context.Context, roomCode, userID string) bool {
	if s.limiter == nil {
		return true
	}
	decision, err := s.limiter.Allow(
		ctx, "danmaku", roomCode+":"+userID, 10, 10*time.Second,
	)
	return err == nil && decision.Allowed
}

func mediaFingerprint(room Room, episode int) string {
	identity := room.SourceURL
	if room.MediaSourceID != "" {
		identity = room.MediaSourceID + "\x00" + room.MediaPath
	}
	digest := sha256.Sum256([]byte(identity + "\x00" + strconv.Itoa(episode)))
	return hex.EncodeToString(digest[:])
}
