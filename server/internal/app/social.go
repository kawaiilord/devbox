package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
)

func (s *Server) searchSocialUsers(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) > 100 {
		writeError(w, http.StatusBadRequest, errors.New("search query is too long"))
		return
	}
	profiles, err := s.repo.SearchSocialProfiles(r.Context(), user.ID, query, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not search users"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"users": profiles}, Msg: "ok"})
}

func (s *Server) socialProfile(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	profile, err := s.repo.GetSocialProfile(r.Context(), user.ID, strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: profile, Msg: "ok"})
}

func (s *Server) followSocialUser(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := s.repo.FollowUser(r.Context(), user.ID, strings.TrimSpace(r.PathValue("id"))); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "followed"})
}

func (s *Server) unfollowSocialUser(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := s.repo.UnfollowUser(r.Context(), user.ID, strings.TrimSpace(r.PathValue("id"))); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "unfollowed"})
}

func (s *Server) createDirectConversation(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		UserID string `json:"user_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.UserID = strings.TrimSpace(request.UserID)
	privacy, err := s.repo.GetPrivacy(r.Context(), request.UserID)
	if err != nil || !privacy.AllowPrivateChat {
		writeError(w, http.StatusForbidden, ErrForbidden)
		return
	}
	conversation, err := s.repo.CreateConversation(r.Context(), user.ID, request.UserID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: conversation, Msg: "ok"})
}

func (s *Server) listDirectConversations(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	items, err := s.repo.ListConversations(r.Context(), user.ID, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load conversations"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"conversations": items}, Msg: "ok"})
}

func (s *Server) directMessages(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, errors.New("invalid conversation"))
		return
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	messages, err := s.repo.ListDirectMessages(r.Context(), user.ID, id, before, limit)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if len(messages) > 0 {
		_ = s.repo.MarkConversationRead(r.Context(), user.ID, id, messages[len(messages)-1].ID)
	}
	unread, _ := s.repo.UnreadDirectCount(r.Context(), user.ID)
	s.pushUnread(r.Context(), user.ID, unread)
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"messages": messages}, Msg: "ok"})
}

func (s *Server) sendDirectMessage(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, errors.New("invalid conversation"))
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
	if !safeModerationText(request.Body, 2000, false) {
		writeError(w, http.StatusBadRequest, errors.New("invalid message"))
		return
	}
	peer, err := s.repo.ConversationPeer(r.Context(), user.ID, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	privacy, err := s.repo.GetPrivacy(r.Context(), peer)
	if err != nil || !privacy.AllowPrivateChat {
		writeError(w, http.StatusForbidden, ErrForbidden)
		return
	}
	message, recipient, err := s.repo.AddDirectMessage(r.Context(), user.ID, id, request.Body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	payload, _ := json.Marshal(message)
	s.emitEnvelope(r.Context(), Envelope{Type: "social.message", Room: socialChannel(recipient), Seq: message.ID, TS: message.CreatedAt, From: user.ID, Payload: payload})
	unread, _ := s.repo.UnreadDirectCount(r.Context(), recipient)
	s.pushUnread(r.Context(), recipient, unread)
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: message, Msg: "sent"})
}

func (s *Server) directUnread(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	count, err := s.repo.UnreadDirectCount(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]int{"count": count}, Msg: "ok"})
}

func (s *Server) socialSocketTicket(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var ticket string
	if s.redis != nil {
		ticket, err = s.redis.IssueTicket(r.Context(), "@SOCIAL", user)
	} else {
		ticket, err = s.store.IssueIdentityTicket(user)
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("could not issue social socket ticket"))
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: map[string]string{"ticket": ticket}, Msg: "created"})
}

func (s *Server) socialSocket(w http.ResponseWriter, r *http.Request) {
	var user User
	var err error
	if s.redis != nil {
		user, err = s.redis.ConsumeTicket(r.Context(), r.URL.Query().Get("ticket"), "@SOCIAL")
	} else {
		user, err = s.store.ConsumeIdentityTicket(r.URL.Query().Get("ticket"))
	}
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	options := &websocket.AcceptOptions{OriginPatterns: s.options.AllowedOrigins}
	for _, origin := range s.options.AllowedOrigins {
		if strings.TrimSpace(origin) == "*" {
			options.OriginPatterns = nil
			options.InsecureSkipVerify = true
			break
		}
	}
	conn, err := websocket.Accept(w, r, options)
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	client := &socketClient{user: user, send: make(chan []byte, 32)}
	unsubscribe := s.hub.Subscribe(socialChannel(user.ID), client)
	defer func() { unsubscribe(); cancel(); _ = conn.Close(websocket.StatusNormalClosure, "goodbye") }()
	count, _ := s.repo.UnreadDirectCount(ctx, user.ID)
	payload, _ := json.Marshal(map[string]int{"count": count})
	client.send <- mustJSON(Envelope{Type: "social.unread", Room: socialChannel(user.ID), TS: time.Now().UnixMilli(), From: "server", Payload: payload})
	go s.writeSocket(ctx, conn, client)
	conn.SetReadLimit(1024)
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func (s *Server) pushUnread(ctx context.Context, userID string, count int) {
	payload, _ := json.Marshal(map[string]int{"count": count})
	s.emitEnvelope(ctx, Envelope{Type: "social.unread", Room: socialChannel(userID), TS: time.Now().UnixMilli(), From: "server", Payload: payload})
}
func socialChannel(userID string) string { return "@social:" + userID }
