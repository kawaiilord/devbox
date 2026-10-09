package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxWatchSeconds = 30 * 24 * 60 * 60

func (s *Server) addFavorite(w http.ResponseWriter, r *http.Request) {
	if s.sources == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("media sources unavailable"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if !s.enforceRateLimit(w, r, "favorite-write", user.ID, 120, time.Hour) {
		return
	}
	var request struct {
		SourceID    string `json:"source_id"`
		MediaPath   string `json:"media_path"`
		Title       string `json:"title"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.SourceID = strings.TrimSpace(request.SourceID)
	request.Title = strings.TrimSpace(request.Title)
	request.ContentType = strings.TrimSpace(request.ContentType)
	if request.SourceID == "" || !validLibraryText(request.Title, 256) ||
		!validLibraryTextOptional(request.ContentType, 128) || request.Size < 0 || request.Size > 1<<60 {
		writeError(w, http.StatusBadRequest, errors.New("invalid favorite"))
		return
	}
	ticket, err := s.sources.PrepareMediaTicket(
		r.Context(), user.ID, request.SourceID, request.MediaPath,
	)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeStoreError(w, err)
		} else {
			writeError(w, http.StatusBadGateway, err)
		}
		return
	}
	favorite, err := s.repo.UpsertFavorite(r.Context(), Favorite{
		UserID: user.ID, SourceID: request.SourceID, MediaPath: ticket.Path,
		Title: request.Title, ContentType: request.ContentType, Size: request.Size,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not save favorite"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: favorite, Msg: "saved"})
}

func (s *Server) listFavorites(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	before, limit := libraryPage(r)
	favorites, err := s.repo.ListFavorites(r.Context(), user.ID, before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load favorites"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{
		Code: 0, Data: map[string]any{"favorites": favorites}, Msg: "ok",
	})
}

func (s *Server) deleteFavorite(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, errors.New("invalid favorite identifier"))
		return
	}
	if err := s.repo.DeleteFavorite(r.Context(), user.ID, id); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "deleted"})
}

func (s *Server) updateWatchProgress(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if !s.enforceRateLimit(w, r, "watch-progress", user.ID, 180, time.Hour) {
		return
	}
	var request struct {
		Duration float64 `json:"duration_seconds"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !validWatchSeconds(request.Duration) {
		writeError(w, http.StatusBadRequest, errors.New("invalid media duration"))
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
	room = s.syncRedisPlayback(r.Context(), room)
	position := math.Max(0, math.Min(room.Playback.Position, maxWatchSeconds))
	duration := request.Duration
	completed := duration > 0 && position/duration >= 0.95
	companionCount := watchCompanionCount(room, user.ID)
	if s.redis != nil {
		if presence, presenceErr := s.redis.Presence(r.Context(), code); presenceErr == nil {
			companionCount = presence.OnlineCount - 1
			if companionCount < 0 {
				companionCount = 0
			}
		}
	}
	mediaIdentity := room.SourceURL
	if room.MediaSourceID != "" {
		mediaIdentity = room.MediaSourceID + "\x00" + room.MediaPath
	}
	digest := sha256.Sum256([]byte(mediaIdentity))
	record := WatchRecord{
		UserID: user.ID, MediaKey: hex.EncodeToString(digest[:]), Title: room.Name,
		Position: position, Duration: duration, Episode: room.Playback.Episode,
		Completed: completed, CompanionCount: companionCount, RoomCode: room.Code,
	}
	if room.OwnerID == user.ID && room.MediaSourceID != "" {
		if _, sourceErr := s.repo.GetMediaSource(r.Context(), user.ID, room.MediaSourceID); sourceErr == nil {
			record.SourceID = room.MediaSourceID
			record.MediaPath = room.MediaPath
		}
	}
	record, err = s.repo.UpsertWatchRecord(r.Context(), record)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not save watch progress"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: record, Msg: "saved"})
}

func (s *Server) listWatchHistory(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	before, limit := libraryPage(r)
	records, err := s.repo.ListWatchRecords(r.Context(), user.ID, before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load watch history"))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{
		Code: 0, Data: map[string]any{"records": records}, Msg: "ok",
	})
}

func (s *Server) deleteWatchRecord(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, errors.New("invalid watch record identifier"))
		return
	}
	if err := s.repo.DeleteWatchRecord(r.Context(), user.ID, id); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Msg: "deleted"})
}

func (s *Server) publicWatchActivity(w http.ResponseWriter, r *http.Request) {
	viewer, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	targetID := strings.TrimSpace(r.PathValue("id"))
	if targetID == "" || len(targetID) > 128 {
		writeError(w, http.StatusBadRequest, errors.New("invalid user identifier"))
		return
	}
	if _, err := s.repo.UserByID(r.Context(), targetID); err != nil {
		writeError(w, http.StatusNotFound, ErrNotFound)
		return
	}
	if targetID != viewer.ID {
		privacy, err := s.repo.GetPrivacy(r.Context(), targetID)
		if err != nil || !privacy.ShowWatchActivity {
			writeError(w, http.StatusForbidden, ErrForbidden)
			return
		}
		blocked, err := s.repo.UsersBlocked(r.Context(), viewer.ID, targetID)
		if err != nil || blocked {
			writeError(w, http.StatusForbidden, ErrForbidden)
			return
		}
	}
	_, limit := libraryPage(r)
	if limit > 20 {
		limit = 20
	}
	records, err := s.repo.ListWatchRecords(r.Context(), targetID, 0, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not load watch activity"))
		return
	}
	activity := make([]WatchActivity, 0, len(records))
	for _, record := range records {
		activity = append(activity, WatchActivity{
			Title: record.Title, Episode: record.Episode, Completed: record.Completed,
			CompanionCount: record.CompanionCount, WatchedAt: record.WatchedAt,
		})
	}
	writeJSON(w, http.StatusOK, apiResponse{
		Code: 0, Data: map[string]any{"activity": activity}, Msg: "ok",
	})
}

func libraryPage(r *http.Request) (int64, int) {
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	return before, limit
}

func watchCompanionCount(room Room, userID string) int {
	count := 0
	for _, member := range room.Members {
		if member.UserID != userID {
			count++
		}
	}
	return count
}

func validWatchSeconds(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= maxWatchSeconds
}

func validLibraryText(value string, limit int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) > 0 &&
		utf8.RuneCountInString(value) <= limit &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func validLibraryTextOptional(value string, limit int) bool {
	return value == "" || validLibraryText(value, limit)
}
