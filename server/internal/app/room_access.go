package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type roomSettingsInput struct {
	ExpectedVersion int64    `json:"expected_version"`
	Visibility      string   `json:"visibility"`
	Description     string   `json:"description"`
	Category        string   `json:"category"`
	Tags            []string `json:"tags"`
	AllowGuests     bool     `json:"allow_guests"`
	Password        *string  `json:"password"`
	AutoNext        *bool    `json:"auto_next"`
}

func requestPeerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func applyRoomSettings(f *RoomFeatures, in roomSettingsInput) error {
	if in.Visibility == "" {
		in.Visibility = "private"
	}
	if in.Visibility != "private" && in.Visibility != "public" {
		return errors.New("房间可见性无效")
	}
	if utf8.RuneCountInString(in.Description) > 500 || utf8.RuneCountInString(in.Category) > 40 || len(in.Tags) > 8 {
		return errors.New("简介、分类或标签过长")
	}
	tags := []string{}
	seen := map[string]bool{}
	for _, tag := range in.Tags {
		tag = strings.TrimSpace(tag)
		if utf8.RuneCountInString(tag) > 30 {
			return errors.New("标签最多 30 个字符")
		}
		if tag != "" && !seen[tag] {
			tags = append(tags, tag)
			seen[tag] = true
		}
	}
	if in.Password != nil {
		if *in.Password == "" {
			f.PasswordHash = ""
		} else {
			if len(*in.Password) < 6 || len(*in.Password) > 128 {
				return errors.New("房间密码需为 6–128 字节")
			}
			hash, err := hashPassword(*in.Password)
			if err != nil {
				return err
			}
			f.PasswordHash = hash
		}
	}
	f.Visibility = in.Visibility
	f.Description = strings.TrimSpace(in.Description)
	f.Category = strings.TrimSpace(in.Category)
	f.Tags = tags
	f.AllowGuests = in.AllowGuests
	if in.AutoNext != nil {
		f.AutoNext = *in.AutoNext
	}
	return nil
}

func initialRoomFeatures(room Room, in roomSettingsInput) (RoomFeatures, error) {
	f := defaultRoomFeatures()
	if err := applyRoomSettings(&f, in); err != nil {
		return f, err
	}
	source := PlaylistSource{ID: mustRandomString(10), Label: "原始片源", OwnerID: room.OwnerID, MediaSourceID: room.MediaSourceID, MediaPath: room.MediaPath, URL: room.SourceURL}
	item := PlaylistEntry{ID: mustRandomString(10), Title: room.Name, Sources: []PlaylistSource{source}, SelectedSourceID: source.ID}
	f.Playlist = []PlaylistEntry{item}
	f.ActiveItemID = item.ID
	return f, nil
}

func (s *Server) discoverPublicRooms(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	q := normalizeDiscoveryQuery(r.URL.Query().Get("q"), r.URL.Query().Get("category"), r.URL.Query().Get("tag"), offset, limit)
	q.Limit++
	rooms, err := s.repo.DiscoverRooms(r.Context(), q)
	if err != nil {
		writeError(w, 503, errors.New("公开房间暂不可用"))
		return
	}
	hasMore := len(rooms) == q.Limit
	if hasMore {
		rooms = rooms[:len(rooms)-1]
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"rooms": rooms, "has_more": hasMore, "next_offset": q.Offset + len(rooms)}})
}

func (s *Server) updateRoomSettings(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	var in roomSettingsInput
	if err = decodeJSON(r, &in); err != nil {
		writeError(w, 400, errors.New("房间设置格式无效"))
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	err = s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		room, f, err := s.authorizedRoom(ctx, code, user)
		if err != nil {
			return err
		}
		if room.OwnerID != user.ID {
			return ErrForbidden
		}
		if f.Version != in.ExpectedVersion {
			return ErrRoomConflict
		}
		if err := applyRoomSettings(&f, in); err != nil {
			return err
		}
		expected := f.Version
		f.Version++
		if err = s.repo.SaveRoomFeatures(ctx, code, expected, f, nil); err != nil {
			return err
		}
		if !f.AllowGuests {
			members := []Member{}
			for _, m := range room.Members {
				if f.role(room, m.UserID) == "guest" {
					if err = s.repo.DeleteRoomMember(ctx, code, m.UserID); err != nil {
						return err
					}
					s.emitMemberRemoval(ctx, code, m.UserID)
				} else {
					members = append(members, m)
				}
			}
			room.Members = members
			_, sequence, _ := s.store.Snapshot(code)
			s.store.UpsertAuthoritativeRoom(room, sequence)
		}
		return s.publishFeatureChange(ctx, room)
	})
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	s.getRoom(w, r)
}

func (s *Server) updateRoomMember(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	var in struct {
		ExpectedVersion int64            `json:"expected_version"`
		Role            string           `json:"role"`
		Permissions     *RoomPermissions `json:"permissions"`
		Unban           bool             `json:"unban"`
	}
	if err = decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	target := r.PathValue("userID")
	err = s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		room, f, err := s.authorizedRoom(ctx, code, user)
		if err != nil {
			return err
		}
		if room.OwnerID != user.ID || target == room.OwnerID {
			return ErrForbidden
		}
		if f.Version != in.ExpectedVersion {
			return ErrRoomConflict
		}
		if in.Unban {
			ids := []string{}
			for _, id := range f.BannedIDs {
				if id != target {
					ids = append(ids, id)
				}
			}
			f.BannedIDs = ids
		} else {
			found := false
			for _, m := range room.Members {
				if m.UserID == target {
					found = true
				}
			}
			if !found {
				return ErrNotFound
			}
			if f.role(room, target) == "guest" {
				if in.Role != "guest" {
					return errors.New("访客身份不能转换为注册成员")
				}
			}
			if in.Role != "member" && in.Role != "moderator" && in.Role != "guest" {
				return errors.New("成员角色无效")
			}
			if f.Roles == nil {
				f.Roles = map[string]string{}
			}
			if f.Permissions == nil {
				f.Permissions = map[string]RoomPermissions{}
			}
			f.Roles[target] = in.Role
			if in.Permissions == nil {
				delete(f.Permissions, target)
			} else {
				f.Permissions[target] = *in.Permissions
			}
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

func (s *Server) removeRoomMember(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	target := r.PathValue("userID")
	err = s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		room, f, err := s.authorizedRoom(ctx, code, user)
		if err != nil {
			return err
		}
		if room.OwnerID != user.ID || target == room.OwnerID {
			return ErrForbidden
		}
		if !f.blocked(target) {
			f.BannedIDs = append(f.BannedIDs, target)
		}
		expected := f.Version
		f.Version++
		if err = s.repo.SaveRoomFeatures(ctx, code, expected, f, nil); err != nil {
			return err
		}
		if err = s.repo.DeleteRoomMember(ctx, code, target); err != nil {
			return err
		}
		members := []Member{}
		for _, m := range room.Members {
			if m.UserID != target {
				members = append(members, m)
			}
		}
		room.Members = members
		_, sequence, _ := s.store.Snapshot(code)
		s.store.UpsertAuthoritativeRoom(room, sequence)
		s.emitMemberRemoval(ctx, code, target)
		return s.publishFeatureChange(ctx, room)
	})
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	s.getRoom(w, r)
}

func (s *Server) emitMemberRemoval(ctx context.Context, code, userID string) {
	s.hub.Disconnect(code, userID)
	payload, _ := json.Marshal(map[string]string{"user_id": userID})
	s.emitEnvelope(ctx, Envelope{Type: "room.member_removed", Room: code, From: "server", Payload: payload})
}

func (s *Server) joinRoomMember(ctx context.Context, code string, user User, password string) (Room, error) {
	var result Room
	err := s.withRoomMutation(ctx, code, func(ctx context.Context) error {
		if err := s.refreshRedisRoom(ctx, code); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		room, err := s.store.Room(code)
		if err != nil {
			return err
		}
		if room.Closed {
			return ErrRoomClosed
		}
		if room.ExpiresAt <= time.Now().UnixMilli() {
			return ErrExpired
		}
		f, err := s.repo.GetRoomFeatures(ctx, code)
		if err != nil {
			return err
		}
		if f.blocked(user.ID) {
			return ErrForbidden
		}
		if user.GuestRoomCode != "" && (!f.AllowGuests || user.GuestRoomCode != code) {
			return ErrForbidden
		}
		member := false
		for _, m := range room.Members {
			if m.UserID == user.ID {
				member = true
			}
		}
		if !member && f.PasswordHash != "" && !verifyPassword(password, f.PasswordHash) {
			return errors.New("房间密码不正确")
		}
		room, err = s.store.JoinRoom(code, user)
		if err != nil {
			return err
		}
		for _, m := range room.Members {
			if m.UserID == user.ID {
				if err = s.repo.SaveMember(ctx, code, m); err != nil {
					return err
				}
				break
			}
		}
		if user.GuestRoomCode != "" && f.Roles[user.ID] != "guest" {
			if f.Roles == nil {
				f.Roles = map[string]string{}
			}
			f.Roles[user.ID] = "guest"
			expected := f.Version
			f.Version++
			if err = s.repo.SaveRoomFeatures(ctx, code, expected, f, nil); err != nil {
				return err
			}
		}
		if err = s.publishFeatureChange(ctx, room); err != nil {
			return err
		}
		result = room
		return nil
	})
	return result, err
}

func (s *AuthService) createGuest(ctx context.Context, code, name string, expiresAt int64, device DeviceInfo) (Session, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 32 {
		return Session{}, errors.New("访客昵称需为 2–32 个字符")
	}
	device, hash, err := normalizeDevice(device)
	if err != nil {
		return Session{}, err
	}
	now := s.now()
	expiry := time.UnixMilli(expiresAt)
	if max := now.Add(6 * time.Hour); expiry.After(max) {
		expiry = max
	}
	user := User{ID: "guest-" + mustRandomString(12), DisplayName: name, SessionVersion: 1, GuestRoomCode: code, GuestExpiresAt: expiry.UnixMilli()}
	stored := user
	stored.Email = "guest-" + mustRandomString(20) + "@sameframe.invalid"
	if err = s.repository.CreateUser(ctx, AccountRecord{User: stored, PasswordHash: "!guest", CreatedAt: now}); err != nil {
		return Session{}, err
	}
	if err = s.repository.BindDevice(ctx, user.ID, hash, device); err != nil {
		return Session{}, err
	}
	access, err := s.tokens.issueAccessUntil(user, hash, expiry)
	if err != nil {
		return Session{}, err
	}
	return Session{AccessToken: access, TokenType: "bearer", ExpiresIn: int64(expiry.Sub(now).Seconds()), User: user}, nil
}

func (s *Server) joinRoomAsGuest(w http.ResponseWriter, r *http.Request) {
	if !s.enforceRateLimit(w, r, "guest-create", requestPeerIP(r), 10, time.Hour) {
		return
	}
	var in struct {
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if decodeJSON(r, &in) != nil {
		writeError(w, 400, errors.New("访客信息格式无效"))
		return
	}
	if len(in.Password) > 128 {
		writeError(w, 400, errors.New("房间密码过长"))
		return
	}
	device, err := deviceFromRequest(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	if err = s.refreshRedisRoom(r.Context(), code); err != nil && !errors.Is(err, ErrNotFound) {
		writeRoomFeatureError(w, err)
		return
	}
	room, err := s.store.Room(code)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	f, err := s.repo.GetRoomFeatures(r.Context(), code)
	if err != nil || !f.AllowGuests || room.Closed || room.ExpiresAt <= time.Now().UnixMilli() {
		writeError(w, 403, errors.New("此房间未开放访客访问"))
		return
	}
	if f.PasswordHash != "" && !verifyPassword(in.Password, f.PasswordHash) {
		writeError(w, 403, errors.New("房间密码不正确"))
		return
	}
	session, err := s.auth.createGuest(r.Context(), code, in.DisplayName, room.ExpiresAt, device)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	room, err = s.joinRoomMember(r.Context(), code, session.User, in.Password)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	room, err = s.clientRoom(r.Context(), room, session.User.ID)
	if err != nil {
		writeRoomFeatureError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, apiResponse{Code: 0, Data: map[string]any{"session": session, "room": room}})
}

func decodeOptionalRoomPassword(r *http.Request) (string, error) {
	var in struct {
		Password string `json:"password"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeJSON(r, &in); err != nil && !errors.Is(err, io.EOF) {
			return "", errors.New("房间密码格式无效")
		}
	}
	if len(in.Password) > 128 {
		return "", errors.New("房间密码过长")
	}
	return in.Password, nil
}
