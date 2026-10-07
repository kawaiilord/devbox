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
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Options struct {
	Address        string
	AllowedOrigins []string
	Logger         *slog.Logger
	Repository     Repository
	Auth           *AuthService
	InitialRooms   []Room
	AllowDemoAuth  bool
	Redis          *RedisCoordinator
}

type Server struct {
	options Options
	store   *Store
	hub     *Hub
	repo    Repository
	auth    *AuthService
	redis   *RedisCoordinator
	http    *http.Server
	start   sync.Once
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
	if options.Repository == nil {
		options.Repository = NewMemoryRepository()
	}
	if options.Auth == nil {
		tokens, _ := NewTokenManager("development-only-secret-change-me-now", "sameframe-test")
		options.Auth = NewAuthService(options.Repository, tokens)
	}
	store := NewStore()
	store.RestoreRooms(options.InitialRooms)
	s := &Server{
		options: options, store: store, hub: NewHub(), repo: options.Repository,
		auth: options.Auth, redis: options.Redis,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/config", s.config)
	mux.HandleFunc("GET /api/v1/clock", s.clock)
	mux.HandleFunc("POST /api/v1/auth/register", s.register)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/refresh", s.refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("POST /api/v1/session/demo", s.demoSession)
	mux.HandleFunc("GET /api/v1/users/me", s.currentUser)
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
	s.StartBackground(context.Background())
	s.options.Logger.Info("sameframe server listening", "address", s.options.Address)
	return s.http.ListenAndServe()
}

func (s *Server) StartBackground(ctx context.Context) {
	s.start.Do(func() {
		go s.snapshotLoop(ctx)
		if s.redis != nil {
			ready := make(chan struct{})
			go s.consumeRedisEvents(ctx, ready)
			select {
			case <-ready:
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				s.options.Logger.Warn("timed out waiting for Redis event subscription")
			}
		}
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]string{"status": "ok"}, Msg: "ok"})
}

func (s *Server) config(w http.ResponseWriter, _ *http.Request) {
	features := map[string]bool{
		"room": true, "direct_source": true, "chat": false, "voice": false,
		"multi_node_realtime": s.redis != nil, "presence": s.redis != nil,
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{
		"maintenance_mode":     false,
		"features":             features,
		"snapshot_interval_ms": 3000,
	}, Msg: "ok"})
}

func (s *Server) clock(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]int64{"server_time": time.Now().UnixMilli()}, Msg: "ok"})
}

func (s *Server) demoSession(w http.ResponseWriter, r *http.Request) {
	if !s.options.AllowDemoAuth {
		writeError(w, http.StatusNotFound, ErrNotFound)
		return
	}
	var request struct {
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := s.auth.CreateDemo(r.Context(), request.DisplayName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: session, Msg: "created"})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := s.auth.Register(r.Context(), request.Email, request.DisplayName, request.Password)
	if errors.Is(err, ErrEmailExists) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: session, Msg: "created"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := s.auth.Login(r.Context(), request.Email, request.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, ErrInvalidCredentials)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: session, Msg: "ok"})
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := s.auth.Refresh(r.Context(), request.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, ErrInvalidRefresh)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: session, Msg: "ok"})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.auth.Logout(r.Context(), request.RefreshToken); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("logout failed"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "logged out"})
}

func (s *Server) currentUser(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: user, Msg: "ok"})
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
	if err := s.repo.SaveRoom(r.Context(), room); err != nil {
		s.options.Logger.Error("persist room", "error", err, "room", room.Code)
		writeError(w, http.StatusInternalServerError, errors.New("could not persist room"))
		return
	}
	if s.redis != nil {
		if err := s.redis.InitializeRoom(r.Context(), room); err != nil {
			s.options.Logger.Error("initialize Redis room", "error", err, "room", room.Code)
			writeError(w, http.StatusServiceUnavailable, errors.New("realtime state unavailable"))
			return
		}
		state, sequence, stateErr := s.roomStateForBroadcast(r.Context(), room.Code)
		if stateErr != nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("realtime state unavailable"))
			return
		}
		s.broadcastRoomState(r.Context(), state, sequence)
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: room, Msg: "created"})
}

func (s *Server) joinRoom(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	if err := s.refreshRedisRoom(r.Context(), code); err != nil && !errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusServiceUnavailable, errors.New("realtime state unavailable"))
		return
	}
	room, err := s.store.JoinRoom(code, user)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	for _, member := range room.Members {
		if member.UserID == user.ID {
			if err := s.repo.SaveMember(r.Context(), room.Code, member); err != nil {
				s.options.Logger.Error("persist room member", "error", err, "room", room.Code, "user", user.ID)
				writeError(w, http.StatusInternalServerError, errors.New("could not persist room membership"))
				return
			}
			break
		}
	}
	if s.redis != nil {
		if err := s.redis.CacheRoom(r.Context(), room); err != nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("realtime state unavailable"))
			return
		}
	}
	state, sequence, snapshotErr := s.roomStateForBroadcast(r.Context(), room.Code)
	if snapshotErr == nil {
		s.broadcastRoomState(r.Context(), state, sequence)
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: room, Msg: "joined"})
}

func (s *Server) getRoom(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	if err := s.refreshRedisRoom(r.Context(), code); err != nil && !errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusServiceUnavailable, errors.New("realtime state unavailable"))
		return
	}
	room, err := s.store.GetRoom(code, user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	room = s.syncRedisPlayback(r.Context(), room)
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: room, Msg: "ok"})
}

func (s *Server) socketTicket(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	if err := s.refreshRedisRoom(r.Context(), code); err != nil && !errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusServiceUnavailable, errors.New("realtime state unavailable"))
		return
	}
	if _, err := s.store.GetRoom(code, user.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	var ticket string
	if s.redis != nil {
		ticket, err = s.redis.IssueTicket(r.Context(), code, user)
	} else {
		ticket, err = s.store.IssueSocketTicket(code, user)
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: map[string]string{"ticket": ticket}, Msg: "created"})
}

func (s *Server) roomSocket(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(r.PathValue("code"))
	var user User
	var err error
	if s.redis != nil {
		user, err = s.redis.ConsumeTicket(r.Context(), r.URL.Query().Get("ticket"), code)
	} else {
		user, err = s.store.ConsumeSocketTicket(r.URL.Query().Get("ticket"), code)
	}
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := s.refreshRedisRoom(r.Context(), code); err != nil && !errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusServiceUnavailable, errors.New("realtime state unavailable"))
		return
	}
	room, err := s.store.GetRoom(code, user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	room = s.syncRedisPlayback(r.Context(), room)
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
	connectionID := mustRandomString(12)
	if s.redis != nil {
		if err := s.redis.TouchPresence(ctx, code, connectionID, user); err != nil {
			s.options.Logger.Warn("add room presence", "error", err, "room", code, "user", user.ID)
		} else {
			s.publishPresence(ctx, code)
			go s.presenceHeartbeat(ctx, code, connectionID, user)
		}
	}
	defer func() {
		unsubscribe()
		cancel()
		if s.redis != nil {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
			if s.redis.RemovePresence(cleanupCtx, code, connectionID) == nil {
				s.publishPresence(cleanupCtx, code)
			}
			cleanupCancel()
		}
		_ = conn.Close(websocket.StatusNormalClosure, "goodbye")
	}()

	initial, _ := json.Marshal(room)
	client.send <- mustJSON(Envelope{Type: "room.state", Room: code, Seq: 0, TS: time.Now().UnixMilli(), Payload: initial})
	if s.redis != nil {
		if presence, presenceErr := s.redis.Presence(ctx, code); presenceErr == nil {
			payload, _ := json.Marshal(presence)
			client.send <- mustJSON(Envelope{
				Type: "room.presence", Room: code, TS: time.Now().UnixMilli(), From: "server", Payload: payload,
			})
		}
	}
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
		currentRoom, membershipErr := s.store.GetRoom(code, user.ID)
		if membershipErr != nil || currentRoom.OwnerID != user.ID {
			s.sendSocketError(client, code, ErrForbidden)
			continue
		}
		var updated Room
		var serverSeq int64
		var applyErr error
		if s.redis != nil {
			var playback Playback
			playback, serverSeq, applyErr = s.redis.ApplyControl(ctx, code, user.ID, envelope.Seq, control)
			if applyErr == nil {
				updated, applyErr = s.store.ApplyAuthoritativePlayback(code, playback, serverSeq)
			}
		} else {
			updated, serverSeq, applyErr = s.store.ApplyControl(code, user, envelope.Seq, control)
		}
		if applyErr != nil {
			s.sendSocketError(client, code, applyErr)
			continue
		}
		if persistErr := s.repo.UpdatePlayback(ctx, code, updated.Playback); persistErr != nil {
			s.options.Logger.Error("persist playback", "error", persistErr, "room", code)
			s.sendSocketError(client, code, errors.New("playback persistence failed"))
			continue
		}
		s.broadcastSnapshot(ctx, updated, serverSeq, user.ID)
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
				if s.redis != nil {
					playback, seq, acquired, err := s.redis.Snapshot(ctx, code)
					if err != nil || !acquired {
						continue
					}
					room, err := s.store.ApplyAuthoritativePlayback(code, playback, seq)
					if err == nil {
						s.broadcastSnapshot(ctx, room, seq, "server")
					}
					continue
				}
				room, seq, err := s.store.Snapshot(code)
				if err == nil {
					s.broadcastSnapshot(ctx, room, seq, "server")
				}
			}
		}
	}
}

func (s *Server) broadcastSnapshot(ctx context.Context, room Room, seq int64, from string) {
	payload, _ := json.Marshal(room.Playback)
	s.emitEnvelope(ctx, Envelope{
		Type: "playback.snapshot", Room: room.Code, Seq: seq,
		TS: time.Now().UnixMilli(), From: from, Payload: payload,
	})
}

func (s *Server) broadcastRoomState(ctx context.Context, room Room, seq int64) {
	payload, _ := json.Marshal(room)
	s.emitEnvelope(ctx, Envelope{
		Type: "room.state", Room: room.Code, Seq: seq,
		TS: time.Now().UnixMilli(), From: "server", Payload: payload,
	})
}

func (s *Server) emitEnvelope(ctx context.Context, envelope Envelope) {
	if s.redis != nil {
		if err := s.redis.Publish(ctx, envelope); err == nil {
			return
		} else {
			s.options.Logger.Warn("publish realtime event", "error", err, "type", envelope.Type)
		}
	}
	s.hub.Broadcast(envelope.Room, envelope)
}

func (s *Server) consumeRedisEvents(ctx context.Context, ready chan<- struct{}) {
	for ctx.Err() == nil {
		pubsub, err := s.redis.Subscribe(ctx)
		if err != nil {
			s.options.Logger.Warn("subscribe realtime events", "error", err)
			if !waitForRetry(ctx, time.Second) {
				return
			}
			continue
		}
		if ready != nil {
			close(ready)
			ready = nil
		}
		channel := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				pubsub.Close()
				return
			case message, ok := <-channel:
				if !ok {
					pubsub.Close()
					if !waitForRetry(ctx, time.Second) {
						return
					}
					goto reconnect
				}
				var envelope Envelope
				if json.Unmarshal([]byte(message.Payload), &envelope) != nil {
					continue
				}
				s.applyRedisEnvelope(envelope)
				s.hub.Broadcast(envelope.Room, envelope)
			}
		}
	reconnect:
	}
}

func (s *Server) applyRedisEnvelope(envelope Envelope) {
	switch envelope.Type {
	case "playback.snapshot":
		var playback Playback
		if json.Unmarshal(envelope.Payload, &playback) == nil {
			_, _ = s.store.ApplyAuthoritativePlayback(envelope.Room, playback, envelope.Seq)
		}
	case "room.state":
		var room Room
		if json.Unmarshal(envelope.Payload, &room) == nil {
			s.store.UpsertAuthoritativeRoom(room, envelope.Seq)
		}
	}
}

func (s *Server) roomStateForBroadcast(ctx context.Context, code string) (Room, int64, error) {
	if s.redis == nil {
		return s.store.Snapshot(code)
	}
	sequence, err := s.redis.NextSequence(ctx, code)
	if err != nil {
		return Room{}, 0, err
	}
	playback, _, err := s.redis.Playback(ctx, code)
	if err != nil {
		return Room{}, 0, err
	}
	room, err := s.store.ApplyAuthoritativePlayback(code, playback, sequence)
	return room, sequence, err
}

func (s *Server) syncRedisPlayback(ctx context.Context, room Room) Room {
	if s.redis == nil {
		return room
	}
	playback, sequence, err := s.redis.Playback(ctx, room.Code)
	if err != nil {
		return room
	}
	updated, err := s.store.ApplyAuthoritativePlayback(room.Code, playback, sequence)
	if err != nil {
		return room
	}
	return updated
}

func (s *Server) refreshRedisRoom(ctx context.Context, code string) error {
	if s.redis == nil {
		return nil
	}
	room, sequence, err := s.redis.Room(ctx, code)
	if err != nil {
		return err
	}
	s.store.UpsertAuthoritativeRoom(room, sequence)
	return nil
}

func (s *Server) presenceHeartbeat(ctx context.Context, code, connectionID string, user User) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.redis.TouchPresence(ctx, code, connectionID, user); err != nil {
				s.options.Logger.Warn("refresh room presence", "error", err, "room", code)
			}
		}
	}
}

func (s *Server) publishPresence(ctx context.Context, code string) {
	presence, err := s.redis.Presence(ctx, code)
	if err != nil {
		return
	}
	payload, _ := json.Marshal(presence)
	s.emitEnvelope(ctx, Envelope{
		Type: "room.presence", Room: code, TS: time.Now().UnixMilli(), From: "server", Payload: payload,
	})
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
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
	return s.auth.AuthenticateAccess(r.Context(), strings.TrimSpace(strings.TrimPrefix(auth, prefix)))
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
