package app

import (
	"errors"
	"net/http"
	"strconv"
	"time"
)

// Single-viewer playback uses the same encrypted source binding and revocable
// ticket as room playback. No room or room membership is created.
func (s *Server) watchCatalog(w http.ResponseWriter, r *http.Request) {
	user, ok := s.catalogUser(w, r)
	if !ok {
		return
	}
	if s.sources == nil || s.redis == nil {
		writeError(w, 503, errors.New("播放服务暂不可用"))
		return
	}
	if !s.enforceRateLimit(w, r, "catalog-watch", user.ID, 60, time.Minute) {
		return
	}
	var selection CatalogSelection
	if err := decodeJSON(r, &selection); err != nil {
		writeError(w, 400, err)
		return
	}
	selection.AllEpisodes = false
	items, active, err := s.catalog.playlist(r.Context(), user, selection)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	chosen := items[active].Sources[0]
	ticket, err := s.sources.PrepareMediaVariant(r.Context(), user.ID, chosen.MediaSourceID, chosen.MediaPath, chosen.VariantID)
	if err != nil {
		writeError(w, 502, err)
		return
	}
	ticket.ViewerID, ticket.ViewerSessionVersion = user.ID, user.SessionVersion
	raw, expires, err := s.redis.IssueMediaTicket(r.Context(), ticket, 5*time.Minute)
	if err != nil {
		writeError(w, 503, errors.New("暂时无法开始播放，请重试"))
		return
	}
	writeJSON(w, 200, apiResponse{Data: map[string]any{"url": s.mediaTicketURL(raw, ticket), "expires_at": expires.UnixMilli()}})
}

func (s *Server) ownedRooms(w http.ResponseWriter, r *http.Request) {
	user, ok := s.catalogUser(w, r)
	if !ok {
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 10000 {
			writeError(w, 400, errors.New("房间页码无效"))
			return
		}
	}
	const limit = 20
	rows, total, err := s.repo.AdminFindRooms(r.Context(), AdminQuery{OwnerID: user.ID, Status: "active", Offset: offset, Limit: limit})
	if err != nil {
		writeError(w, 503, errors.New("暂时无法读取你的房间，请重试"))
		return
	}
	items := []map[string]any{}
	for _, row := range rows {
		if row.Room.OwnerID != user.ID || row.Room.Closed || roomAdminStatus(row) != "active" {
			continue
		}
		items = append(items, map[string]any{"code": row.Room.Code, "name": row.Room.Name, "created_at": row.Room.CreatedAt, "member_count": len(row.Room.Members)})
	}
	writeJSON(w, 200, apiResponse{Data: map[string]any{"rooms": items, "has_more": offset+len(rows) < total, "next_offset": offset + len(rows)}})
}
