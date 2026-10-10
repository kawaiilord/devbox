package app

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

func (s *Server) validatePlaylistSource(ctx context.Context, user User, in PlaylistSource) (PlaylistSource, error) {
	in.ID = mustRandomString(10)
	in.OwnerID = user.ID
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" {
		in.Label = "片源"
	}
	if utf8.RuneCountInString(in.Label) > 80 {
		return PlaylistSource{}, errors.New("片源名称过长")
	}
	if len(in.MediaPath) > 4096 || len(in.VariantID) > 128 {
		return PlaylistSource{}, errors.New("媒体路径或画质标识过长")
	}
	if in.MediaSourceID != "" {
		in.URL = ""
		if s.sources == nil {
			return PlaylistSource{}, errors.New("媒体源服务未配置")
		}
		ticket, err := s.sources.PrepareMediaVariant(ctx, user.ID, in.MediaSourceID, in.MediaPath, in.VariantID)
		if err != nil {
			return PlaylistSource{}, err
		}
		in.IsLive = ticket.IsLive
	} else {
		in.MediaPath = ""
		in.VariantID = ""
		in.URL = strings.TrimSpace(in.URL)
		u, err := url.Parse(in.URL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || len(in.URL) > 8192 {
			return PlaylistSource{}, errors.New("请输入完整的 HTTP(S) 视频链接")
		}
	}
	return in, nil
}

func (s *Server) selectionForItem(ctx context.Context, room Room, f RoomFeatures, item PlaylistEntry, keepPosition bool) (RoomMediaSelection, error) {
	var source PlaylistSource
	found := false
	for _, candidate := range item.Sources {
		if candidate.ID == item.SelectedSourceID {
			source = candidate
			found = true
			break
		}
	}
	if !found {
		return RoomMediaSelection{}, errors.New("请选择可用片源")
	}
	if source.MediaSourceID != "" {
		if s.sources == nil {
			return RoomMediaSelection{}, errors.New("媒体源服务未配置")
		}
		if _, err := s.sources.PrepareMediaVariant(ctx, source.OwnerID, source.MediaSourceID, source.MediaPath, source.VariantID); err != nil {
			return RoomMediaSelection{}, err
		}
	}
	playback := projected(room.Playback, time.Now())
	playback.SourceVersion++
	playback.Episode = item.Episode
	if !keepPosition {
		playback.Position = 0
		playback.Playing = true
	}
	return RoomMediaSelection{MediaSourceID: source.MediaSourceID, MediaPath: source.MediaPath, SourceURL: source.URL, Playback: playback}, nil
}

func (s *Server) getRoomPlaylist(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	room, f, err := s.authorizedRoom(r.Context(), r.PathValue("code"), user)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: f.view(room, user.ID)})
}

func (s *Server) addPlaylistItems(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	var in struct {
		ExpectedVersion int64           `json:"expected_version"`
		Items           []PlaylistEntry `json:"items"`
	}
	if err = decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if len(in.Items) == 0 || len(in.Items) > 50 {
		writeError(w, 400, errors.New("每次可添加 1–50 集"))
		return
	}
	room, f, err := s.authorizedRoom(r.Context(), code, user)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	if !f.grants(room, user.ID).Playlist {
		writeError(w, 403, ErrForbidden)
		return
	}
	items := make([]PlaylistEntry, 0, len(in.Items))
	for _, item := range in.Items {
		item.Title = strings.TrimSpace(item.Title)
		if item.Title == "" || utf8.RuneCountInString(item.Title) > 256 || len(item.Sources) == 0 || len(item.Sources) > 8 {
			writeError(w, 400, errors.New("影片标题或片源数量无效"))
			return
		}
		item.ID = mustRandomString(10)
		item.SelectedSourceID = ""
		for i, source := range item.Sources {
			validated, err := s.validatePlaylistSource(r.Context(), user, source)
			if err != nil {
				writeRoomFeatureError(w, err)
				return
			}
			item.Sources[i] = validated
		}
		item.SelectedSourceID = item.Sources[0].ID
		items = append(items, item)
	}
	err = s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		room, f, err := s.authorizedRoom(ctx, code, user)
		if err != nil {
			return err
		}
		if !f.grants(room, user.ID).Playlist {
			return ErrForbidden
		}
		if f.Version != in.ExpectedVersion {
			return ErrRoomConflict
		}
		if len(f.Playlist)+len(items) > 500 {
			return errors.New("每个房间最多保存 500 集")
		}
		nextEpisode := 0
		for _, item := range f.Playlist {
			nextEpisode = max(nextEpisode, item.Episode+1)
		}
		for i := range items {
			items[i].Episode = nextEpisode + i
		}
		f.Playlist = append(f.Playlist, items...)
		if f.ActiveItemID == "" {
			f.ActiveItemID = items[0].ID
			selection, err := s.selectionForItem(ctx, room, f, items[0], false)
			if err != nil {
				return err
			}
			return s.saveSelectedMedia(ctx, room, f, selection)
		}
		expected := f.Version
		f.Version++
		if err = s.repo.SaveRoomFeatures(ctx, code, expected, f, nil); err != nil {
			return err
		}
		return s.publishFeatureChange(ctx, room)
	})
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	s.getRoom(w, r)
}

func (s *Server) reorderPlaylist(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	var in struct {
		ExpectedVersion int64    `json:"expected_version"`
		Order           []string `json:"order"`
	}
	if err = decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	err = s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		room, f, err := s.authorizedRoom(ctx, code, user)
		if err != nil {
			return err
		}
		if !f.grants(room, user.ID).Playlist {
			return ErrForbidden
		}
		if f.Version != in.ExpectedVersion {
			return ErrRoomConflict
		}
		if len(in.Order) != len(f.Playlist) {
			return errors.New("片单顺序不完整")
		}
		byID := map[string]PlaylistEntry{}
		for _, item := range f.Playlist {
			byID[item.ID] = item
		}
		ordered := make([]PlaylistEntry, 0, len(in.Order))
		for _, id := range in.Order {
			item, ok := byID[id]
			if !ok {
				return errors.New("片单包含重复或未知影片")
			}
			ordered = append(ordered, item)
			delete(byID, id)
		}
		f.Playlist = ordered
		expected := f.Version
		f.Version++
		if err = s.repo.SaveRoomFeatures(ctx, code, expected, f, nil); err != nil {
			return err
		}
		return s.publishFeatureChange(ctx, room)
	})
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	s.getRoom(w, r)
}

func (s *Server) deletePlaylistItem(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	id := r.PathValue("itemID")
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if err = decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	err = s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		room, f, err := s.authorizedRoom(ctx, code, user)
		if err != nil {
			return err
		}
		if !f.grants(room, user.ID).Playlist {
			return ErrForbidden
		}
		if f.Version != in.ExpectedVersion {
			return ErrRoomConflict
		}
		items := []PlaylistEntry{}
		index := -1
		for i, item := range f.Playlist {
			if item.ID == id {
				index = i
			} else {
				items = append(items, item)
			}
		}
		if index < 0 {
			return ErrNotFound
		}
		f.Playlist = items
		if f.ActiveItemID == id {
			if len(items) == 0 {
				f.ActiveItemID = ""
				playback := room.Playback
				playback.Position = 0
				playback.Playing = false
				playback.SourceVersion++
				playback.PositionTS = time.Now().UnixMilli()
				return s.saveSelectedMedia(ctx, room, f, RoomMediaSelection{Playback: playback})
			}
			item := items[min(index, len(items)-1)]
			f.ActiveItemID = item.ID
			selection, err := s.selectionForItem(ctx, room, f, item, false)
			if err != nil {
				return err
			}
			return s.saveSelectedMedia(ctx, room, f, selection)
		}
		expected := f.Version
		f.Version++
		if err = s.repo.SaveRoomFeatures(ctx, code, expected, f, nil); err != nil {
			return err
		}
		return s.publishFeatureChange(ctx, room)
	})
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	s.getRoom(w, r)
}

func (s *Server) addPlaylistSource(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	id := r.PathValue("itemID")
	var in struct {
		ExpectedVersion int64          `json:"expected_version"`
		Source          PlaylistSource `json:"source"`
	}
	if err = decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	room, f, err := s.authorizedRoom(r.Context(), code, user)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	if !f.grants(room, user.ID).Playlist {
		writeError(w, 403, ErrForbidden)
		return
	}
	source, err := s.validatePlaylistSource(r.Context(), user, in.Source)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	err = s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		room, f, err := s.authorizedRoom(ctx, code, user)
		if err != nil {
			return err
		}
		if !f.grants(room, user.ID).Playlist {
			return ErrForbidden
		}
		if f.Version != in.ExpectedVersion {
			return ErrRoomConflict
		}
		found := false
		for i := range f.Playlist {
			if f.Playlist[i].ID == id {
				if len(f.Playlist[i].Sources) >= 8 {
					return errors.New("每集最多 8 个片源")
				}
				f.Playlist[i].Sources = append(f.Playlist[i].Sources, source)
				found = true
				break
			}
		}
		if !found {
			return ErrNotFound
		}
		expected := f.Version
		f.Version++
		if err = s.repo.SaveRoomFeatures(ctx, code, expected, f, nil); err != nil {
			return err
		}
		return s.publishFeatureChange(ctx, room)
	})
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	s.getRoom(w, r)
}

func (s *Server) selectPlaylistMedia(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	var in struct {
		ExpectedVersion int64   `json:"expected_version"`
		ItemID          string  `json:"item_id"`
		SourceID        string  `json:"source_id"`
		VariantID       *string `json:"variant_id"`
		Direction       string  `json:"direction"`
		Auto            bool    `json:"auto"`
		ExpectedItemID  string  `json:"expected_item_id"`
	}
	if err = decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	err = s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		room, f, err := s.authorizedRoom(ctx, code, user)
		if err != nil {
			return err
		}
		if !f.grants(room, user.ID).Playback {
			return ErrForbidden
		}
		if f.Version != in.ExpectedVersion {
			return ErrRoomConflict
		}
		if in.Auto && (!f.AutoNext || user.ID != room.OwnerID || in.ExpectedItemID != f.ActiveItemID) {
			return ErrRoomConflict
		}
		index := -1
		if in.Direction != "" {
			for i, item := range f.Playlist {
				if item.ID == f.ActiveItemID {
					index = i
					break
				}
			}
			if index < 0 {
				return ErrNotFound
			}
			switch in.Direction {
			case "next":
				index++
			case "previous":
				index--
			default:
				return errors.New("切集方向无效")
			}
		} else {
			for i, item := range f.Playlist {
				if item.ID == in.ItemID {
					index = i
					break
				}
			}
		}
		if index < 0 || index >= len(f.Playlist) {
			return errors.New("没有更多剧集")
		}
		item := f.Playlist[index]
		if in.SourceID != "" {
			found := false
			for _, source := range item.Sources {
				if source.ID == in.SourceID {
					found = true
				}
			}
			if !found {
				return ErrNotFound
			}
			item.SelectedSourceID = in.SourceID
		}
		if in.VariantID != nil {
			if len(*in.VariantID) > 128 {
				return errors.New("画质标识过长")
			}
			for i := range item.Sources {
				if item.Sources[i].ID == item.SelectedSourceID {
					item.Sources[i].VariantID = *in.VariantID
				}
			}
		}
		selection, err := s.selectionForItem(ctx, room, f, item, item.ID == f.ActiveItemID)
		if err != nil {
			return err
		}
		f.Playlist[index] = item
		f.ActiveItemID = item.ID
		return s.saveSelectedMedia(ctx, room, f, selection)
	})
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	s.getRoom(w, r)
}

func (s *Server) roomMediaVariants(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	_, f, err := s.authorizedRoom(r.Context(), r.PathValue("code"), user)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	itemID := r.URL.Query().Get("item_id")
	if itemID == "" {
		itemID = f.ActiveItemID
	}
	sourceID := r.URL.Query().Get("source_id")
	for _, item := range f.Playlist {
		if item.ID != itemID {
			continue
		}
		if sourceID == "" {
			sourceID = item.SelectedSourceID
		}
		for _, source := range item.Sources {
			if source.ID != sourceID {
				continue
			}
			variants := []MediaVariant{{ID: "original", Label: "原画", Protocol: "file"}}
			if source.MediaSourceID != "" {
				if s.sources == nil {
					writeError(w, 503, errors.New("媒体源服务未配置"))
					return
				}
				variants, err = s.sources.Variants(r.Context(), source.OwnerID, source.MediaSourceID, source.MediaPath)
				if err != nil {
					writeRoomFeatureError(w, err)
					return
				}
			}
			writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"variants": variants, "selected": source.VariantID}})
			return
		}
	}
	writeStoreError(w, ErrNotFound)
}
