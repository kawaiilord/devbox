package app

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Server) createReviewUpload(w http.ResponseWriter, r *http.Request) {
	if s.options.Objects == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("object storage unavailable"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ext, ok := reviewImageExtension(strings.ToLower(strings.TrimSpace(request.ContentType)))
	if !ok || request.Size < 1 || request.Size > 10<<20 {
		writeError(w, http.StatusBadRequest, errors.New("review image must be JPEG, PNG, or WebP and at most 10 MiB"))
		return
	}
	key := "reviews/" + user.ID + "/" + mustRandomString(18) + ext
	expiry := 10 * time.Minute
	url, err := s.options.Objects.PresignPut(r.Context(), key, expiry)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("could not sign upload"))
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: ObjectUpload{ObjectKey: key, UploadURL: url, ExpiresAt: time.Now().Add(expiry).UnixMilli()}, Msg: "created"})
}
func (s *Server) upsertReview(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		TargetType string   `json:"target_type"`
		TargetID   string   `json:"target_id"`
		Title      string   `json:"title"`
		Rating     int      `json:"rating"`
		Content    string   `json:"content"`
		ImageKeys  []string `json:"image_keys"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.TargetType = strings.ToLower(strings.TrimSpace(request.TargetType))
	request.TargetID = strings.TrimSpace(request.TargetID)
	request.Title = strings.TrimSpace(request.Title)
	request.Content = strings.TrimSpace(request.Content)
	if (request.TargetType != "movie" && request.TargetType != "tv" && request.TargetType != "media") || !validLibraryText(request.TargetID, 256) || !validLibraryText(request.Title, 256) || request.Rating < 1 || request.Rating > 10 || !safeModerationText(request.Content, 5000, false) || len(request.ImageKeys) > 4 {
		writeError(w, http.StatusBadRequest, errors.New("invalid review"))
		return
	}
	seen := map[string]bool{}
	for _, key := range request.ImageKeys {
		if seen[key] || !strings.HasPrefix(key, "reviews/"+user.ID+"/") || s.options.Objects == nil {
			writeError(w, http.StatusForbidden, ErrForbidden)
			return
		}
		seen[key] = true
		info, statErr := s.options.Objects.Stat(r.Context(), key)
		_, validContentType := reviewImageExtension(strings.ToLower(strings.TrimSpace(info.ContentType)))
		if statErr != nil || info.Size < 1 || info.Size > 10<<20 || !reviewImageKey(key) || !validContentType {
			writeError(w, http.StatusBadRequest, errors.New("review image is missing or invalid"))
			return
		}
	}
	review, removedKeys, err := s.repo.UpsertReview(r.Context(), Review{UserID: user.ID, TargetType: request.TargetType, TargetID: request.TargetID, Title: request.Title, Rating: request.Rating, Content: request.Content, ImageKeys: append([]string(nil), request.ImageKeys...)})
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not save review"))
		return
	}
	if s.options.Objects != nil {
		for _, key := range removedKeys {
			_ = s.options.Objects.Delete(r.Context(), key)
		}
	}
	s.hydrateReviewImages(r.Context(), &review)
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: review, Msg: "saved"})
}
func (s *Server) listReviews(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("target_type")))
	id := strings.TrimSpace(r.URL.Query().Get("target_id"))
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	if (kind != "movie" && kind != "tv" && kind != "media") || id == "" {
		writeError(w, http.StatusBadRequest, errors.New("invalid review target"))
		return
	}
	items, err := s.repo.ListReviews(r.Context(), user.ID, kind, id, before, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for i := range items {
		s.hydrateReviewImages(r.Context(), &items[i])
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"reviews": items}, Msg: "ok"})
}
func (s *Server) deleteReview(w http.ResponseWriter, r *http.Request) {
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
	keys, err := s.repo.DeleteReview(r.Context(), user.ID, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if s.options.Objects != nil {
		for _, key := range keys {
			_ = s.options.Objects.Delete(r.Context(), key)
		}
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "deleted"})
}
func (s *Server) addReviewComment(w http.ResponseWriter, r *http.Request) {
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
		Body string `json:"body"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.Body = strings.TrimSpace(request.Body)
	if !safeModerationText(request.Body, 2000, false) {
		writeError(w, http.StatusBadRequest, errors.New("invalid comment"))
		return
	}
	comment, err := s.repo.AddReviewComment(r.Context(), ReviewComment{ReviewID: id, UserID: user.ID, Body: request.Body})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: comment, Msg: "created"})
}
func (s *Server) listReviewComments(w http.ResponseWriter, r *http.Request) {
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
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	items, err := s.repo.ListReviewComments(r.Context(), user.ID, id, before, 100)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"comments": items}, Msg: "ok"})
}
func (s *Server) hydrateReviewImages(ctx context.Context, review *Review) {
	review.ImageURLs = []string{}
	if s.options.Objects == nil {
		return
	}
	for _, key := range review.ImageKeys {
		if value, err := s.options.Objects.PresignGet(ctx, key, 15*time.Minute); err == nil {
			review.ImageURLs = append(review.ImageURLs, value)
		}
	}
}
func reviewImageExtension(contentType string) (string, bool) {
	switch contentType {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	default:
		return "", false
	}
}
func reviewImageKey(key string) bool {
	switch strings.ToLower(filepath.Ext(key)) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	default:
		return false
	}
}
