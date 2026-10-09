package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
)

type Options struct {
	Address              string
	AllowedOrigins       []string
	Logger               *slog.Logger
	Repository           Repository
	Auth                 *AuthService
	InitialRooms         []Room
	AllowDemoAuth        bool
	Redis                *RedisCoordinator
	RequireVerifiedEmail bool
	Sources              *MediaSourceManager
	PublicBaseURL        string
}

type Server struct {
	options Options
	store   *Store
	hub     *Hub
	repo    Repository
	auth    *AuthService
	redis   *RedisCoordinator
	limiter *RateLimiter
	sources *MediaSourceManager
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
		sources: options.Sources,
	}
	if options.Redis != nil {
		s.limiter = options.Redis.RateLimiter()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/config", s.config)
	mux.HandleFunc("GET /api/v1/clock", s.clock)
	mux.HandleFunc("POST /api/v1/auth/register", s.register)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/refresh", s.refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("POST /api/v1/auth/email/verify-request", s.verificationRequest)
	mux.HandleFunc("POST /api/v1/auth/email/verify", s.verifyEmail)
	mux.HandleFunc("POST /api/v1/auth/password/request", s.passwordResetRequest)
	mux.HandleFunc("POST /api/v1/auth/password/reset", s.passwordReset)
	mux.HandleFunc("POST /api/v1/session/demo", s.demoSession)
	mux.HandleFunc("GET /api/v1/users/me", s.currentUser)
	mux.HandleFunc("GET /api/v1/devices", s.listDevices)
	mux.HandleFunc("DELETE /api/v1/devices/{id}", s.revokeDevice)
	mux.HandleFunc("GET /api/v1/privacy", s.getPrivacy)
	mux.HandleFunc("PATCH /api/v1/privacy", s.updatePrivacy)
	mux.HandleFunc("GET /api/v1/blocks", s.listBlocks)
	mux.HandleFunc("POST /api/v1/blocks/{user_id}", s.blockUser)
	mux.HandleFunc("DELETE /api/v1/blocks/{user_id}", s.unblockUser)
	mux.HandleFunc("POST /api/v1/reports", s.createReport)
	mux.HandleFunc("GET /api/v1/admin/reports", s.adminReports)
	mux.HandleFunc("POST /api/v1/admin/reports/{id}/resolve", s.resolveReport)
	mux.HandleFunc("POST /api/v1/admin/rooms/{code}/close", s.adminCloseRoom)
	mux.HandleFunc("POST /api/v1/admin/devices/{hash}/ban", s.adminBanDevice)
	mux.HandleFunc("GET /api/v1/admin/audit", s.adminAudit)
	mux.HandleFunc("POST /api/v1/sources/webdav", s.createWebDAVSource)
	mux.HandleFunc("GET /api/v1/sources", s.listMediaSources)
	mux.HandleFunc("DELETE /api/v1/sources/{id}", s.deleteMediaSource)
	mux.HandleFunc("GET /api/v1/sources/{id}/files", s.browseMediaSource)
	mux.HandleFunc("POST /api/v1/sources/{id}/ticket", s.issueMediaTicket)
	mux.HandleFunc("GET /media/{ticket}", s.proxyMedia)
	mux.HandleFunc("HEAD /media/{ticket}", s.proxyMedia)
	mux.HandleFunc("POST /api/v1/rooms", s.createRoom)
	mux.HandleFunc("POST /api/v1/rooms/{code}/join", s.joinRoom)
	mux.HandleFunc("POST /api/v1/rooms/{code}/socket-ticket", s.socketTicket)
	mux.HandleFunc("POST /api/v1/rooms/{code}/media-ticket", s.issueRoomMediaTicket)
	mux.HandleFunc("GET /api/v1/rooms/{code}/messages", s.roomMessages)
	mux.HandleFunc("GET /api/v1/rooms/{code}/subtitles", s.roomSubtitles)
	mux.HandleFunc("POST /api/v1/rooms/{code}/subtitle-ticket", s.issueRoomSubtitleTicket)
	mux.HandleFunc("GET /api/v1/rooms/{code}", s.getRoom)
	mux.HandleFunc("GET /ws/v1/rooms/{code}", s.roomSocket)
	s.http = &http.Server{
		Addr:              options.Address,
		Handler:           s.withCORS(s.withRequestLog(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 * 1024,
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
		"room": true, "direct_source": true, "chat": true, "voice": false,
		"multi_node_realtime": s.redis != nil, "presence": s.redis != nil,
		"device_management": true, "email_verification": s.options.RequireVerifiedEmail,
		"media_sources":      s.sources != nil,
		"external_subtitles": s.sources != nil,
		"privacy_controls":   true,
		"moderation":         true,
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
	device, err := deviceFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := s.auth.CreateDemo(r.Context(), request.DisplayName, device)
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
	device, err := deviceFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.enforceRateLimit(w, r, "register-source", sourceIdentity(r), 20, time.Hour) ||
		!s.enforceRateLimit(w, r, "register-device", device.ID, 5, time.Hour) {
		return
	}
	session, err := s.auth.Register(
		r.Context(), request.Email, request.DisplayName, request.Password, device,
	)
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
	device, err := deviceFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	normalizedEmail := strings.ToLower(strings.TrimSpace(request.Email))
	if !s.enforceRateLimit(w, r, "login-source", clientIdentity(r), 10, 15*time.Minute) ||
		!s.enforceRateLimit(w, r, "login-account", normalizedEmail, 20, 15*time.Minute) {
		return
	}
	session, err := s.auth.Login(r.Context(), request.Email, request.Password, device)
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
	device, err := deviceFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.enforceRateLimit(w, r, "refresh-device", device.ID, 30, time.Minute) ||
		!s.enforceRateLimit(w, r, "refresh-source", sourceIdentity(r), 100, time.Minute) {
		return
	}
	session, err := s.auth.Refresh(r.Context(), request.RefreshToken, device)
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

func (s *Server) verificationRequest(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if !s.enforceRateLimit(w, r, "verify-email", user.ID, 5, time.Hour) {
		return
	}
	if err := s.auth.RequestEmailVerification(r.Context(), user); err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("verification message unavailable"))
		return
	}
	writeJSON(w, http.StatusAccepted, apiResponse{Code: 0, Msg: "verification requested"})
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.enforceRateLimit(w, r, "verify-token", clientIdentity(r), 15, 15*time.Minute) {
		return
	}
	if err := s.auth.VerifyEmail(r.Context(), request.Token); err != nil {
		writeError(w, http.StatusBadRequest, ErrInvalidActionToken)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "email verified"})
}

func (s *Server) passwordResetRequest(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	normalizedEmail := strings.ToLower(strings.TrimSpace(request.Email))
	if !s.enforceRateLimit(w, r, "password-request-account", normalizedEmail, 5, time.Hour) ||
		!s.enforceRateLimit(w, r, "password-request-source", sourceIdentity(r), 20, time.Hour) {
		return
	}
	s.auth.RequestPasswordReset(r.Context(), request.Email)
	writeJSON(w, http.StatusAccepted, apiResponse{
		Code: 0, Msg: "if the account exists, a reset message will be sent",
	})
}

func (s *Server) passwordReset(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.enforceRateLimit(w, r, "password-reset", clientIdentity(r), 10, 30*time.Minute) {
		return
	}
	if err := s.auth.ResetPassword(r.Context(), request.Token, request.Password); err != nil {
		if errors.Is(err, ErrInvalidActionToken) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "password updated"})
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	devices, err := s.repo.UserDevices(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load devices"))
		return
	}
	currentHash := hashDeviceID(r.Header.Get("X-Device-ID"))
	for index := range devices {
		devices[index].Current = devices[index].ID == currentHash
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"devices": devices}, Msg: "ok"})
}

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	deviceHash := strings.ToLower(strings.TrimSpace(r.PathValue("id")))
	if len(deviceHash) != 64 {
		writeError(w, http.StatusBadRequest, errors.New("invalid device"))
		return
	}
	if deviceHash == hashDeviceID(r.Header.Get("X-Device-ID")) {
		writeError(w, http.StatusBadRequest, errors.New("current device cannot be revoked here"))
		return
	}
	if err := s.repo.RevokeUserDevice(r.Context(), user.ID, deviceHash); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "device revoked"})
}

func (s *Server) createWebDAVSource(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("media sources unavailable"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if s.options.RequireVerifiedEmail && !user.EmailVerified {
		writeError(w, http.StatusForbidden, errors.New("email verification required"))
		return
	}
	if !s.enforceRateLimit(w, r, "source-create", user.ID, 10, time.Hour) {
		return
	}
	var request struct {
		Name     string `json:"name"`
		BaseURL  string `json:"base_url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	source, err := s.sources.CreateWebDAV(
		r.Context(), user, request.Name, request.BaseURL, request.Username, request.Password,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: source, Msg: "created"})
}

func (s *Server) listMediaSources(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("media sources unavailable"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	sources, err := s.sources.List(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load media sources"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"sources": sources}, Msg: "ok"})
}

func (s *Server) deleteMediaSource(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("media sources unavailable"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := s.sources.Delete(r.Context(), user.ID, r.PathValue("id")); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "source deleted"})
}

func (s *Server) browseMediaSource(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("media sources unavailable"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if !s.enforceRateLimit(w, r, "source-browse", user.ID, 60, time.Minute) {
		return
	}
	files, err := s.sources.Browse(
		r.Context(), user.ID, r.PathValue("id"), r.URL.Query().Get("path"),
	)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"files": files}, Msg: "ok"})
}

func (s *Server) issueMediaTicket(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil || s.redis == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("media tickets unavailable"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if !s.enforceRateLimit(w, r, "media-ticket", user.ID, 60, time.Minute) {
		return
	}
	var request struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.repo.GetMediaSource(r.Context(), user.ID, r.PathValue("id")); err != nil {
		writeStoreError(w, err)
		return
	}
	if _, err := resolveSourcePath("https://validation.invalid/", request.Path, false); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	raw, expiresAt, err := s.redis.IssueMediaTicket(r.Context(), MediaTicket{
		UserID: user.ID, SourceID: r.PathValue("id"), Path: cleanMediaPath(request.Path),
	}, 5*time.Minute)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("could not issue media ticket"))
		return
	}
	playURL := s.mediaPlaybackURL(raw)
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: map[string]any{
		"url": playURL, "expires_at": expiresAt.UnixMilli(),
	}, Msg: "created"})
}

func (s *Server) issueRoomMediaTicket(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil || s.redis == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("media tickets unavailable"))
		return
	}
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
	hydrated, expiresAt, err := s.issueTicketForRoom(r.Context(), room)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("could not issue room media ticket"))
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: map[string]any{
		"url": hydrated.SourceURL, "expires_at": expiresAt.UnixMilli(),
	}, Msg: "created"})
}

func (s *Server) roomMessages(w http.ResponseWriter, r *http.Request) {
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
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	privacy, err := s.repo.GetPrivacy(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load privacy settings"))
		return
	}
	if !privacy.AllowRoomChat {
		writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"messages": []ChatMessage{}}, Msg: "ok"})
		return
	}
	messages, err := s.repo.ListRoomMessages(r.Context(), code, user.ID, before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load room messages"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"messages": messages}, Msg: "ok"})
}

func (s *Server) roomSubtitles(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("media sources unavailable"))
		return
	}
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
	if room.MediaSourceID == "" {
		writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"files": []MediaFile{}}, Msg: "ok"})
		return
	}
	directory := path.Dir(room.MediaPath)
	files, err := s.sources.Browse(r.Context(), room.OwnerID, room.MediaSourceID, directory)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	subtitles := make([]MediaFile, 0)
	for _, file := range files {
		if !file.IsDirectory && isSubtitlePath(file.Path) {
			subtitles = append(subtitles, file)
		}
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"files": subtitles}, Msg: "ok"})
}

func (s *Server) issueRoomSubtitleTicket(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil || s.redis == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("subtitle tickets unavailable"))
		return
	}
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
	var request struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	clean := cleanMediaPath(request.Path)
	if room.MediaSourceID == "" || !isSubtitlePath(clean) || path.Dir(clean) != path.Dir(room.MediaPath) {
		writeError(w, http.StatusForbidden, errors.New("subtitle is outside the room media directory"))
		return
	}
	raw, expiresAt, err := s.redis.IssueMediaTicket(r.Context(), MediaTicket{
		UserID: room.OwnerID, SourceID: room.MediaSourceID, Path: clean,
	}, 15*time.Minute)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("could not issue subtitle ticket"))
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: map[string]any{
		"url": s.mediaPlaybackURL(raw), "expires_at": expiresAt.UnixMilli(),
	}, Msg: "created"})
}

func isSubtitlePath(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".srt", ".vtt", ".ass", ".ssa":
		return true
	default:
		return false
	}
}

func (s *Server) hydrateRoomMediaURL(ctx context.Context, room Room) (Room, error) {
	if room.MediaSourceID == "" {
		return room, nil
	}
	hydrated, _, err := s.issueTicketForRoom(ctx, room)
	return hydrated, err
}

func (s *Server) issueTicketForRoom(
	ctx context.Context,
	room Room,
) (Room, time.Time, error) {
	if s.redis == nil || s.sources == nil {
		return Room{}, time.Time{}, errors.New("media tickets unavailable")
	}
	if _, err := s.repo.GetMediaSource(ctx, room.OwnerID, room.MediaSourceID); err != nil {
		return Room{}, time.Time{}, err
	}
	raw, expiresAt, err := s.redis.IssueMediaTicket(ctx, MediaTicket{
		UserID: room.OwnerID, SourceID: room.MediaSourceID, Path: room.MediaPath,
	}, 5*time.Minute)
	if err != nil {
		return Room{}, time.Time{}, err
	}
	room.SourceURL = s.mediaPlaybackURL(raw)
	return room, expiresAt, nil
}

func (s *Server) mediaPlaybackURL(raw string) string {
	playURL := "/media/" + raw
	if s.options.PublicBaseURL == "" {
		return playURL
	}
	base, err := url.Parse(s.options.PublicBaseURL)
	if err != nil {
		return playURL
	}
	return base.ResolveReference(&url.URL{Path: playURL}).String()
}

func (s *Server) proxyMedia(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil || s.redis == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("media proxy unavailable"))
		return
	}
	raw := r.PathValue("ticket")
	if len(raw) < 24 || len(raw) > 128 {
		writeError(w, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	ticket, err := s.redis.MediaTicket(r.Context(), raw)
	if err != nil {
		writeError(w, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	rangeHeader := r.Header.Get("Range")
	ifRange := r.Header.Get("If-Range")
	if (rangeHeader != "" && !singleByteRange.MatchString(rangeHeader)) || len(ifRange) > 256 {
		writeError(w, http.StatusRequestedRangeNotSatisfiable, errors.New("invalid range request"))
		return
	}
	response, err := s.sources.Open(
		r.Context(), ticket, r.Method, rangeHeader, ifRange,
	)
	if err != nil {
		writeError(w, http.StatusBadGateway, errors.New("upstream media unavailable"))
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		writeError(w, http.StatusBadGateway, fmt.Errorf("upstream media returned HTTP %d", response.StatusCode))
		return
	}
	for _, header := range []string{
		"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified",
	} {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, response.Body)
}

var singleByteRange = regexp.MustCompile(`^bytes=(?:[0-9]+-[0-9]*|-[0-9]+)$`)

func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if s.options.RequireVerifiedEmail && !user.EmailVerified {
		writeError(w, http.StatusForbidden, errors.New("email verification required"))
		return
	}
	var request struct {
		Name          string `json:"name"`
		SourceURL     string `json:"source_url"`
		MediaSourceID string `json:"media_source_id"`
		MediaPath     string `json:"media_path"`
		MaxMembers    int    `json:"max_members"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var room Room
	if request.MediaSourceID != "" {
		if s.sources == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("media sources unavailable"))
			return
		}
		if _, err := s.repo.GetMediaSource(r.Context(), user.ID, request.MediaSourceID); err != nil {
			writeStoreError(w, err)
			return
		}
		room, err = s.store.CreateMediaRoom(
			user, request.Name, request.MediaSourceID, request.MediaPath, request.MaxMembers,
		)
	} else {
		room, err = s.store.CreateRoom(user, request.Name, request.SourceURL, request.MaxMembers)
	}
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
	clientRoom, err := s.hydrateRoomMediaURL(r.Context(), room)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("could not issue room media ticket"))
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: clientRoom, Msg: "created"})
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
	clientRoom, err := s.hydrateRoomMediaURL(r.Context(), room)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("could not issue room media ticket"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: clientRoom, Msg: "joined"})
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
	room, err = s.hydrateRoomMediaURL(r.Context(), room)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("could not issue room media ticket"))
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
	room, err = s.hydrateRoomMediaURL(r.Context(), room)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("could not issue room media ticket"))
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
		if json.Unmarshal(message, &envelope) != nil {
			continue
		}
		if envelope.Type == "chat.message" {
			s.handleChatMessage(ctx, client, code, user, envelope.Payload)
			continue
		}
		if envelope.Type != "playback.control" {
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

func (s *Server) handleChatMessage(
	ctx context.Context,
	client *socketClient,
	roomCode string,
	user User,
	payload json.RawMessage,
) {
	if _, err := s.store.GetRoom(roomCode, user.ID); err != nil {
		s.sendSocketError(client, roomCode, ErrForbidden)
		return
	}
	privacy, err := s.repo.GetPrivacy(ctx, user.ID)
	if err != nil || !privacy.AllowRoomChat {
		s.sendSocketError(client, roomCode, ErrForbidden)
		return
	}
	var incoming struct {
		Body string `json:"body"`
	}
	if json.Unmarshal(payload, &incoming) != nil {
		s.sendSocketError(client, roomCode, errors.New("invalid chat message"))
		return
	}
	body := strings.TrimSpace(incoming.Body)
	if !utf8.ValidString(body) || utf8.RuneCountInString(body) < 1 ||
		utf8.RuneCountInString(body) > 500 || strings.ContainsRune(body, '\x00') {
		s.sendSocketError(client, roomCode, errors.New("chat message must be 1-500 characters"))
		return
	}
	if !s.chatRateAllowed(ctx, roomCode, user.ID) {
		s.sendSocketError(client, roomCode, errors.New("chat rate limit exceeded"))
		return
	}
	message, err := s.repo.AddRoomMessage(ctx, ChatMessage{
		RoomCode: roomCode, UserID: user.ID, DisplayName: user.DisplayName, Body: body,
	})
	if err != nil {
		s.sendSocketError(client, roomCode, errors.New("chat persistence failed"))
		return
	}
	encoded, _ := json.Marshal(message)
	s.emitEnvelope(ctx, Envelope{
		Type: "chat.message", Room: roomCode, Seq: message.ID,
		TS: message.CreatedAt, From: user.ID, Payload: encoded,
	})
}

func (s *Server) chatRateAllowed(ctx context.Context, roomCode, userID string) bool {
	if s.limiter == nil {
		return true
	}
	decision, err := s.limiter.Allow(
		ctx, "chat", roomCode+":"+userID, 20, 10*time.Second,
	)
	return err == nil && decision.Allowed
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
	s.broadcastLocal(ctx, envelope)
}

func (s *Server) broadcastLocal(ctx context.Context, envelope Envelope) {
	if envelope.Type != "chat.message" {
		s.hub.Broadcast(envelope.Room, envelope)
		return
	}
	s.hub.BroadcastWhere(envelope.Room, envelope, func(recipient User) bool {
		privacy, err := s.repo.GetPrivacy(ctx, recipient.ID)
		if err != nil || !privacy.AllowRoomChat {
			return false
		}
		blocked, err := s.repo.UsersBlocked(ctx, recipient.ID, envelope.From)
		return err == nil && !blocked
	})
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
				s.broadcastLocal(ctx, envelope)
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
	return s.auth.AuthenticateAccess(
		r.Context(),
		strings.TrimSpace(strings.TrimPrefix(auth, prefix)),
		r.Header.Get("X-Device-ID"),
	)
}

func deviceFromRequest(r *http.Request) (DeviceInfo, error) {
	device := DeviceInfo{
		ID: r.Header.Get("X-Device-ID"), Label: r.Header.Get("X-Device-Name"),
		Platform: r.Header.Get("X-Device-Platform"),
	}
	normalized, _, err := normalizeDevice(device)
	return normalized, err
}

func clientIdentity(r *http.Request) string {
	return sourceIdentity(r) + ":" + r.Header.Get("X-Device-ID")
}

func sourceIdentity(r *http.Request) string {
	host := r.RemoteAddr
	if parsedHost, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = parsedHost
	}
	return host
}

func (s *Server) enforceRateLimit(
	w http.ResponseWriter,
	r *http.Request,
	bucket, identity string,
	limit int64,
	window time.Duration,
) bool {
	if s.limiter == nil {
		return true
	}
	decision, err := s.limiter.Allow(r.Context(), bucket, identity, limit, window)
	if err != nil {
		s.options.Logger.Error("rate limiter unavailable", "error", err, "bucket", bucket)
		writeError(w, http.StatusServiceUnavailable, errors.New("rate limiter unavailable"))
		return false
	}
	w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(decision.Remaining, 10))
	if decision.Allowed {
		return true
	}
	retrySeconds := int64(decision.RetryAfter.Round(time.Second) / time.Second)
	if retrySeconds < 1 {
		retrySeconds = 1
	}
	w.Header().Set("Retry-After", strconv.FormatInt(retrySeconds, 10))
	writeError(w, http.StatusTooManyRequests, errors.New("too many requests"))
	return false
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && originAllowed(origin, s.options.AllowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set(
			"Access-Control-Allow-Headers",
			"Authorization, Content-Type, Range, If-Range, X-Device-ID, X-Device-Name, X-Device-Platform",
		)
		w.Header().Set(
			"Access-Control-Expose-Headers",
			"Accept-Ranges, Content-Length, Content-Range, ETag, Last-Modified, Retry-After, X-RateLimit-Remaining",
		)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
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
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrExpired), errors.Is(err, ErrRoomClosed):
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
