package app

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
)

type AdminRoomChange struct {
	Version     int64             `json:"version"`
	Action      string            `json:"action"`
	Name        string            `json:"name"`
	MaxMembers  int               `json:"max_members"`
	Settings    roomSettingsInput `json:"settings"`
	UserID      string            `json:"user_id"`
	Role        string            `json:"role"`
	Permissions *RoomPermissions  `json:"permissions"`
	Reason      string            `json:"reason"`
	Confirm     string            `json:"confirm"`
}

func adminRoomSummary(record AdminRoomRecord) map[string]any {
	room, f := record.Room, record.Features
	live := false
	if source, _, ok := f.activeSource(); ok {
		live = source.IsLive
	}
	return map[string]any{"code": room.Code, "name": room.Name, "owner_id": room.OwnerID, "owner_name": record.OwnerName, "owner_email": record.OwnerEmail, "member_count": len(room.Members), "max_members": room.MaxMembers, "created_at": room.CreatedAt, "expires_at": room.ExpiresAt, "status": roomAdminStatus(record), "reason": f.AdminReason, "version": f.Version, "category": f.Category, "tags": f.Tags, "visibility": f.Visibility, "password_protected": f.PasswordHash != "", "is_live": live}
}
func adminRoomDetails(record AdminRoomRecord) map[string]any {
	data := adminRoomSummary(record)
	room, f := record.Room, record.Features
	settings := f.view(room, room.OwnerID)
	settings.Playlist = nil
	data["settings"] = settings
	data["playback"] = room.Playback
	members := []map[string]any{}
	for _, m := range room.Members {
		_, overridden := f.Permissions[m.UserID]
		members = append(members, map[string]any{"user_id": m.UserID, "display_name": m.DisplayName, "joined_at": m.JoinedAt, "role": f.role(room, m.UserID), "permissions": f.grants(room, m.UserID), "has_override": overridden})
	}
	data["members"] = members
	data["banned_ids"] = f.BannedIDs
	return data
}
func (s *Server) adminRooms(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "room.view"); !ok {
		return
	}
	q := adminQuery(r)
	items, total, err := s.repo.AdminFindRooms(r.Context(), q)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	list := []map[string]any{}
	for _, record := range items {
		list = append(list, adminRoomSummary(record))
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"rooms": list, "total": total, "offset": q.Offset, "limit": q.Limit, "has_more": q.Offset+len(items) < total}})
}
func (s *Server) adminRoom(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "room.view"); !ok {
		return
	}
	item, err := s.repo.AdminReadRoom(r.Context(), strings.ToUpper(r.PathValue("code")))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: adminRoomDetails(item)})
}
func (s *Server) adminChangeRoom(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "room.manage")
	if !ok {
		return
	}
	var in AdminRoomChange
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.Action == "delete" && in.Confirm != "DELETE ROOM" {
		writeError(w, 428, errors.New("请确认将房间移入回收站"))
		return
	}
	item, err := s.changeManagedRoom(r.Context(), strings.ToUpper(r.PathValue("code")), actor, in)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: adminRoomDetails(item)})
}
func (s *Server) changeManagedRoom(ctx context.Context, code string, actor User, in AdminRoomChange) (AdminRoomRecord, error) {
	var result AdminRoomRecord
	if !safeModerationText(in.Reason, 500, true) {
		return result, errors.New("操作原因无效")
	}
	if (in.Action == "ban" || in.Action == "delete" || in.Action == "member_remove") && strings.TrimSpace(in.Reason) == "" {
		return result, errors.New("请填写操作原因")
	}
	err := s.withRoomMutation(ctx, code, func(ctx context.Context) error {
		current, err := s.repo.AdminReadRoom(ctx, code)
		if err != nil {
			return err
		}
		if current.Features.Version != in.Version {
			return ErrAdminConflict
		}
		result = current
		room, f := &result.Room, &result.Features
		state := roomAdminStatus(current)
		if state == "deleted" && in.Action != "restore" {
			return ErrAdminConflict
		}
		switch in.Action {
		case "settings":
			if !safeModerationText(strings.TrimSpace(in.Name), 64, false) || len([]rune(strings.TrimSpace(in.Name))) < 2 {
				return errors.New("房间名称需为 2–64 个字符")
			}
			policy, _, err := s.sitePolicy(ctx)
			if err != nil {
				return err
			}
			if in.MaxMembers < 2 || in.MaxMembers > policy.MaxRoomMembers || in.MaxMembers < len(room.Members) {
				return errors.New("人数上限不得小于现有成员数或超过站点限制")
			}
			if err = applyRoomSettings(f, in.Settings); err != nil {
				return err
			}
			room.Name, room.MaxMembers = strings.TrimSpace(in.Name), in.MaxMembers
			if !f.AllowGuests {
				members := []Member{}
				for _, m := range room.Members {
					if f.role(*room, m.UserID) != "guest" {
						members = append(members, m)
					}
				}
				room.Members = members
			}
		case "ban", "delete":
			room.Closed = true
			f.AdminState = map[string]string{"ban": "banned", "delete": "deleted"}[in.Action]
			f.AdminReason = in.Reason
		case "unban", "restore":
			if (in.Action == "unban" && state != "banned") || (in.Action == "restore" && state != "deleted" && state != "closed") {
				return ErrAdminConflict
			}
			if room.ExpiresAt <= time.Now().UnixMilli() {
				return errors.New("房间已过期，不能恢复")
			}
			room.Closed = false
			f.AdminState = "active"
			f.AdminReason = ""
		case "member_add":
			if room.Closed {
				return ErrRoomClosed
			}
			account, err := s.repo.UserByID(ctx, in.UserID)
			if err != nil {
				return ErrNotFound
			}
			if !accountActive(account.User) || strings.HasPrefix(account.ID, "guest-") || f.blocked(account.ID) {
				return ErrForbidden
			}
			for _, m := range room.Members {
				if m.UserID == in.UserID {
					return errors.New("用户已经在房间内")
				}
			}
			if len(room.Members) >= room.MaxMembers {
				return ErrRoomFull
			}
			room.Members = append(room.Members, Member{UserID: account.ID, DisplayName: account.DisplayName, JoinedAt: time.Now().UnixMilli()})
		case "member_update", "member_remove", "transfer_owner":
			var member *Member
			for i := range room.Members {
				if room.Members[i].UserID == in.UserID {
					member = &room.Members[i]
					break
				}
			}
			if member == nil {
				return ErrNotFound
			}
			if in.UserID == room.OwnerID {
				return ErrForbidden
			}
			if in.Action == "member_update" {
				role := f.role(*room, in.UserID)
				if !slices.Contains([]string{"member", "moderator", "guest"}, in.Role) || (role == "guest" && in.Role != "guest") || (role != "guest" && in.Role == "guest") {
					return errors.New("成员角色无效")
				}
				if f.Roles == nil {
					f.Roles = map[string]string{}
				}
				if f.Permissions == nil {
					f.Permissions = map[string]RoomPermissions{}
				}
				f.Roles[in.UserID] = in.Role
				if in.Permissions == nil {
					delete(f.Permissions, in.UserID)
				} else {
					f.Permissions[in.UserID] = *in.Permissions
				}
			} else if in.Action == "member_remove" {
				members := []Member{}
				for _, m := range room.Members {
					if m.UserID != in.UserID {
						members = append(members, m)
					}
				}
				room.Members = members
				if !f.blocked(in.UserID) {
					f.BannedIDs = append(f.BannedIDs, in.UserID)
				}
			} else {
				account, err := s.repo.UserByID(ctx, in.UserID)
				if err != nil {
					return ErrNotFound
				}
				if !accountActive(account.User) || f.role(*room, in.UserID) == "guest" {
					return ErrForbidden
				}
				if f.Roles == nil {
					f.Roles = map[string]string{}
				}
				f.Roles[room.OwnerID] = "member"
				delete(f.Permissions, room.OwnerID)
				delete(f.Roles, in.UserID)
				delete(f.Permissions, in.UserID)
				room.OwnerID = in.UserID
			}
		case "member_unban":
			f.BannedIDs = slices.DeleteFunc(f.BannedIDs, func(id string) bool { return id == in.UserID })
		default:
			return errors.New("不支持的房间操作")
		}
		f.Version++
		if err = s.repo.AdminSaveRoom(ctx, result, in.Version, actor.ID, in.Action); err != nil {
			return err
		}
		for _, m := range current.Room.Members {
			found := false
			for _, next := range room.Members {
				if next.UserID == m.UserID {
					found = true
					break
				}
			}
			if !found {
				s.emitMemberRemoval(ctx, code, m.UserID)
			}
		}
		persisted, err := s.repo.AdminReadRoom(ctx, code)
		if err != nil {
			return err
		}
		result = persisted
		return s.publishManagedRoom(ctx, persisted.Room)
	})
	return result, err
}
func (s *Server) publishManagedRoom(ctx context.Context, room Room) error {
	_, sequence, _ := s.store.Snapshot(room.Code)
	if s.redis != nil {
		if room.Closed {
			playback, seq, err := s.redis.ApplyControl(ctx, room.Code, "admin:close", time.Now().UnixMicro(), Control{Action: "pause"})
			if err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
			if err == nil {
				room.Playback = playback
				sequence = seq
			}
		}
		if err := s.redis.InitializeRoom(ctx, room); err != nil {
			return err
		}
		var err error
		sequence, err = s.redis.NextSequence(ctx, room.Code)
		if err != nil {
			return err
		}
	}
	s.store.UpsertAuthoritativeRoom(room, sequence)
	s.broadcastRoomState(ctx, room, sequence)
	if room.Closed {
		s.hub.DisconnectRoom(room.Code)
	}
	return nil
}
func (s *Server) adminBatchRooms(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "room.manage")
	if !ok {
		return
	}
	var in struct {
		Action  string `json:"action"`
		Reason  string `json:"reason"`
		Confirm string `json:"confirm"`
		Targets []struct {
			Code    string `json:"code"`
			Version int64  `json:"version"`
		} `json:"targets"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if len(in.Targets) == 0 || len(in.Targets) > 100 || !slices.Contains([]string{"ban", "unban", "delete", "restore"}, in.Action) || !safeModerationText(in.Reason, 500, true) {
		writeError(w, 400, errors.New("请选择 1–100 个房间和有效操作"))
		return
	}
	if in.Action == "delete" && in.Confirm != "DELETE ROOMS" {
		writeError(w, 428, errors.New("请确认批量移入回收站"))
		return
	}
	results := []map[string]any{}
	seen := map[string]bool{}
	for _, target := range in.Targets {
		code := strings.ToUpper(target.Code)
		item := map[string]any{"id": code, "ok": false}
		if seen[code] {
			item["error"] = "重复的房间"
			results = append(results, item)
			continue
		}
		seen[code] = true
		_, err := s.changeManagedRoom(r.Context(), code, actor, AdminRoomChange{Action: in.Action, Version: target.Version, Reason: in.Reason})
		if err != nil {
			item["error"] = err.Error()
		} else {
			item["ok"] = true
		}
		results = append(results, item)
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"results": results}})
}
func (s *Server) adminRoomPlayback(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "room.manage")
	if !ok {
		return
	}
	var control Control
	if err := decodeJSON(r, &control); err != nil {
		writeError(w, 400, err)
		return
	}
	if !slices.Contains([]string{"play", "pause", "seek", "speed"}, control.Action) {
		writeError(w, 400, errors.New("不支持的播放操作"))
		return
	}
	if _, err := validateControl(control); err != nil {
		writeError(w, 400, err)
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	err := s.withRoomMutation(r.Context(), code, func(ctx context.Context) error {
		record, err := s.repo.AdminReadRoom(ctx, code)
		if err != nil {
			return err
		}
		if roomAdminStatus(record) != "active" {
			return ErrRoomClosed
		}
		if source, _, ok := record.Features.activeSource(); ok && source.IsLive && (control.Action == "seek" || control.Action == "speed") {
			return errors.New("直播不支持拖动或倍速")
		}
		if err = s.refreshRedisRoom(ctx, code); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		room := s.syncRedisPlayback(ctx, record.Room)
		if control.SourceVersion == nil || *control.SourceVersion != room.Playback.SourceVersion {
			return ErrAdminConflict
		}
		var updated Room
		var sequence int64
		if s.redis != nil {
			var playback Playback
			playback, sequence, err = s.redis.ApplyControl(ctx, code, "admin:"+actor.ID, time.Now().UnixMicro(), control)
			if err == nil {
				updated, err = s.store.ApplyAuthoritativePlayback(code, playback, sequence)
			}
		} else {
			s.store.UpsertAuthoritativeRoom(room, 0)
			updated, sequence, err = s.store.applyControl(code, User{ID: "admin:" + actor.ID}, time.Now().UnixMicro(), control, true)
		}
		if err != nil {
			return err
		}
		if err = s.repo.UpdatePlayback(ctx, code, updated.Playback); err != nil {
			return err
		}
		if _, err = s.repo.AppendAudit(ctx, AuditEvent{ActorID: actor.ID, Action: "room.playback." + control.Action, TargetType: "room", TargetID: code}); err != nil {
			return err
		}
		s.broadcastSnapshot(ctx, updated, sequence, actor.ID)
		return nil
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Msg: "播放状态已更新"})
}
