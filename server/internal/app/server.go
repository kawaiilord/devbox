package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type Options struct {
	Address        string
	AllowedOrigins []string
	Logger         *slog.Logger
}

type Server struct {
	options Options
	store   *Store
	hub     *Hub
	http    *http.Server
}

type apiResponse struct {
	Code int    `json:"code"`
	Data any    `json:"data,omitempty"`
	Msg  string `json:"msg"`
}

func NewServer(options Options) *Server {
	if options.Address == "" {
		options.Address = ":8080"
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	s := &Server{options: options, store: NewStore(), hub: NewHub()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/config", s.config)
	mux.HandleFunc("GET /api/v1/clock", s.clock)
	mux.HandleFunc("POST /api/v1/session/demo", s.demoSession)
	mux.HandleFunc("POST /api/v1/rooms", s.createRoom)
	mux.HandleFunc("POST /api/v1/rooms/{code}/join", s.joinRoom)
	mux.HandleFunc("POST /api/v1/rooms/{code}/socket-ticket", s.socketTicket)
	mux.HandleFunc("GET /api/v1/rooms/{code}", s.getRoom)
	mux.HandleFunc("GET /ws/v1/rooms/{code}", s.roomSocket)
	s.http = &http.Server{
		Addr:              options.Address,
		Handler:           s.withCORS(s.withRequestLog(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return s
}

func (s *Server) Handler() http.Handler { return s.http.Handler }

func (s *Server) ListenAndServe() error {
	go s.snapshotLoop(context.Background())
	s.options.Logger.Info("sameframe server listening", "address", s.options.Address)
	return s.http.ListenAndServe()
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]string{"status": "ok"}, Msg: "ok"})
}

func (s *Server) config(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{
		"maintenance_mode":     false,
		"features":             map[string]bool{"room": true, "direct_source": true, "chat": false, "voice": false},
		"snapshot_interval_ms": 3000,
	}, Msg: "ok"})
}

func (s *Server) clock(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]int64{"server_time": time.Now().UnixMilli()}, Msg: "ok"})
}

func (s *Server) demoSession(w http.ResponseWriter, r *http.Request) {
	var request struct {
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := s.store.CreateSession(request.DisplayName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: session, Msg: "created"})
}

func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		Name       string `json:"name"`
		SourceURL  string `json:"source_url"`
		MaxMembers int    `json:"max_members"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	room, err := s.store.CreateRoom(user, request.Name, request.SourceURL, request.MaxMembers)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: room, Msg: "created"})
}

func (s *Server) joinRoom(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	room, err := s.store.JoinRoom(r.PathValue("code"), user)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	state, sequence, snapshotErr := s.store.Snapshot(room.Code)
	if snapshotErr == nil {
		s.broadcastRoomState(state, sequence)
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: room, Msg: "joined"})
}

func (s *Server) getRoom(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	room, err := s.store.GetRoom(r.PathValue("code"), user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: room, Msg: "ok"})
}

func (s *Server) socketTicket(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	ticket, err := s.store.IssueSocketTicket(r.PathValue("code"), user)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: map[string]string{"ticket": ticket}, Msg: "created"})
}

func (s *Server) roomSocket(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(r.PathValue("code"))
	user, err := s.store.ConsumeSocketTicket(r.URL.Query().Get("ticket"), code)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	room, err := s.store.GetRoom(code, user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	acceptOptions := &websocket.AcceptOptions{OriginPatterns: s.options.AllowedOrigins}
	for _, origin := range s.options.AllowedOrigins {
		if strings.TrimSpace(origin) == "*" {
			acceptOptions.OriginPatterns = nil
			acceptOptions.InsecureSkipVerify = true
			break
		}
	}
	conn, err := websocket.Accept(w, r, acceptOptions)
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	client := &socketClient{user: user, send: make(chan []byte, 16)}
	unsubscribe := s.hub.Subscribe(code, client)
	defer func() {
		unsubscribe()
		cancel()
		_ = conn.Close(websocket.StatusNormalClosure, "goodbye")
	}()

	initial, _ := json.Marshal(room)
	client.send <- mustJSON(Envelope{Type: "room.state", Room: code, Seq: 0, TS: time.Now().UnixMilli(), Payload: initial})
	go s.writeSocket(ctx, conn, client)

	conn.SetReadLimit(64 * 1024)
	for {
		_, message, readErr := conn.Read(ctx)
		if readErr != nil {
			return
		}
		var envelope Envelope
		if json.Unmarshal(message, &envelope) != nil || envelope.Type != "playback.control" {
			continue
		}
		var control Control
		if json.Unmarshal(envelope.Payload, &control) != nil {
			continue
		}
		updated, serverSeq, applyErr := s.store.ApplyControl(code, user, envelope.Seq, control)
		if applyErr != nil {
			s.sendSocketError(client, code, applyErr)
			continue
		}
		s.broadcastSnapshot(updated, serverSeq, user.ID)
	}
}

func (s *Server) writeSocket(ctx context.Context, conn *websocket.Conn, client *socketClient) {
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-client.send:
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, message)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (s *Server) snapshotLoop(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, code := range s.store.RoomCodes() {
				room, seq, err := s.store.Snapshot(code)
				if err == nil {
					s.broadcastSnapshot(room, seq, "server")
				}
			}
		}
	}
}

func (s *Server) broadcastSnapshot(room Room, seq int64, from string) {
	payload, _ := json.Marshal(room.Playback)
	s.hub.Broadcast(room.Code, Envelope{
		Type: "playback.snapshot", Room: room.Code, Seq: seq,
		TS: time.Now().UnixMilli(), From: from, Payload: payload,
	})
}

func (s *Server) broadcastRoomState(room Room, seq int64) {
	payload, _ := json.Marshal(room)
	s.hub.Broadcast(room.Code, Envelope{
		Type: "room.state", Room: room.Code, Seq: seq,
		TS: time.Now().UnixMilli(), From: "server", Payload: payload,
	})
}

func (s *Server) sendSocketError(client *socketClient, code string, err error) {
	payload, _ := json.Marshal(map[string]string{"message": err.Error()})
	message, _ := json.Marshal(Envelope{Type: "error", Room: code, TS: time.Now().UnixMilli(), Payload: payload})
	select {
	case client.send <- message:
	default:
	}
}

func (s *Server) userFromRequest(r *http.Request) (User, error) {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return User{}, ErrUnauthorized
	}
	return s.store.Authenticate(strings.TrimSpace(strings.TrimPrefix(auth, prefix)))
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && originAllowed(origin, s.options.AllowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		s.options.Logger.Debug("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, apiResponse{Code: status, Msg: err.Error()})
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, err)
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrExpired):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, ErrRoomFull):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}

func originAllowed(origin string, allowed []string) bool {
	for _, pattern := range allowed {
		pattern = strings.TrimSpace(pattern)
		if pattern == "*" || pattern == origin {
			return true
		}
		if strings.HasSuffix(pattern, "*") && strings.HasPrefix(origin, strings.TrimSuffix(pattern, "*")) {
			return true
		}
	}
	return false
}

func mustJSON(envelope Envelope) []byte {
	message, _ := json.Marshal(envelope)
	return message
}
