package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

func adminSearch(haystack, query string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(query))
}
func adminBounds(length int, q AdminQuery) (int, int) {
	start := min(max(q.Offset, 0), length)
	return start, min(start+max(q.Limit, 1), length)
}
func (r *MemoryRepository) AdminFindUsers(_ context.Context, q AdminQuery) ([]ManagedUser, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := []ManagedUser{}
	for _, a := range r.usersByID {
		item := managedUser(a)
		if (q.Status == "" || item.Status == q.Status) && adminSearch(a.ID+" "+a.Email+" "+a.DisplayName, q.Search) {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt == items[j].CreatedAt {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})
	start, end := adminBounds(len(items), q)
	return items[start:end], len(items), nil
}
func (r *MemoryRepository) AdminCreateUser(_ context.Context, account AccountRecord, actor string) (ManagedUser, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !permissionGranted(r.usersByID[actor].User, "user.manage") {
		return ManagedUser{}, ErrForbidden
	}
	if _, ok := r.usersByMail[account.Email]; ok {
		return ManagedUser{}, ErrEmailExists
	}
	account.AccountStatus = "active"
	account.AdminVersion = 1
	r.usersByID[account.ID] = account
	r.usersByMail[account.Email] = account.ID
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "user.create", TargetType: "user", TargetID: account.ID})
	return managedUser(account), nil
}
func (r *MemoryRepository) AdminMutateUser(_ context.Context, id, actor string, change AdminUserChange) (ManagedUser, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.usersByID[id]
	if !ok {
		return ManagedUser{}, ErrNotFound
	}
	if err := applyAdminUserChange(r.usersByID[actor].User, &account, change); err != nil {
		return ManagedUser{}, err
	}
	r.usersByID[id] = account
	if accountChangeRevokes(change.Action) {
		for hash, token := range r.refresh {
			if token.userID == id {
				token.revoked = true
				r.refresh[hash] = token
			}
		}
		for hash, token := range r.actionTokens {
			if token.record.UserID == id {
				token.used = true
				r.actionTokens[hash] = token
			}
		}
	}
	if change.Action == "profile" {
		for code, room := range r.rooms {
			room.Members = append([]Member(nil), room.Members...)
			for i := range room.Members {
				if room.Members[i].UserID == id {
					room.Members[i].DisplayName = account.DisplayName
				}
			}
			r.rooms[code] = room
		}
	}
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "user." + change.Action, TargetType: "user", TargetID: id, Metadata: map[string]any{"reason": change.Reason}})
	return managedUser(account), nil
}
func (r *MemoryRepository) adminRoomLocked(code string) (AdminRoomRecord, error) {
	room, ok := r.rooms[code]
	if !ok {
		return AdminRoomRecord{}, ErrNotFound
	}
	room.Members = append([]Member(nil), room.Members...)
	account := r.usersByID[room.OwnerID]
	f, ok := r.roomFeatures[code]
	if !ok {
		f = defaultRoomFeatures()
	}
	return AdminRoomRecord{Room: room, Features: cloneRoomFeatures(f), OwnerName: account.DisplayName, OwnerEmail: account.Email}, nil
}
func (r *MemoryRepository) AdminFindRooms(_ context.Context, q AdminQuery) ([]AdminRoomRecord, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := []AdminRoomRecord{}
	for code := range r.rooms {
		item, _ := r.adminRoomLocked(code)
		if (q.OwnerID == "" || q.OwnerID == item.Room.OwnerID) && (q.Category == "" || q.Category == item.Features.Category) && (q.Status == "" || q.Status == roomAdminStatus(item)) && adminSearch(item.Room.Code+" "+item.Room.Name+" "+item.OwnerName+" "+item.OwnerEmail, q.Search) {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Room.CreatedAt == items[j].Room.CreatedAt {
			return items[i].Room.Code < items[j].Room.Code
		}
		return items[i].Room.CreatedAt > items[j].Room.CreatedAt
	})
	start, end := adminBounds(len(items), q)
	return items[start:end], len(items), nil
}
func (r *MemoryRepository) AdminReadRoom(_ context.Context, code string) (AdminRoomRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.adminRoomLocked(code)
}
func (r *MemoryRepository) AdminSaveRoom(_ context.Context, record AdminRoomRecord, expected int64, actor, action string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !permissionGranted(r.usersByID[actor].User, "room.manage") {
		return ErrForbidden
	}
	current, err := r.adminRoomLocked(record.Room.Code)
	if err != nil {
		return err
	}
	if current.Features.Version != expected || record.Features.Version != expected+1 {
		return ErrAdminConflict
	}
	owner, ok := r.usersByID[record.Room.OwnerID]
	if !ok || (record.Room.OwnerID != current.Room.OwnerID && !accountActive(owner.User)) {
		return ErrForbidden
	}
	for _, member := range record.Room.Members {
		if _, ok := r.usersByID[member.UserID]; !ok {
			return ErrNotFound
		}
	}
	room := current.Room
	room.Name, room.MaxMembers, room.OwnerID, room.Closed = record.Room.Name, record.Room.MaxMembers, record.Room.OwnerID, record.Room.Closed
	room.AdminEpoch = record.Features.Version
	room.Members = append([]Member(nil), record.Room.Members...)
	if room.Closed {
		room.Playback.Playing = false
	}
	r.rooms[room.Code] = room
	if r.roomFeatures == nil {
		r.roomFeatures = map[string]RoomFeatures{}
	}
	r.roomFeatures[room.Code] = cloneRoomFeatures(record.Features)
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "room.admin." + action, TargetType: "room", TargetID: room.Code, Metadata: map[string]any{"reason": record.Features.AdminReason}})
	return nil
}
func managedSource(source MediaSource, owner User) ManagedSource {
	if source.AdminVersion == 0 {
		source.AdminVersion = 1
	}
	source.CredentialsCiphertext = ""
	return ManagedSource{MediaSource: source, OwnerID: source.UserID, OwnerName: owner.DisplayName, OwnerEmail: owner.Email}
}
func (r *MemoryRepository) AdminFindSources(_ context.Context, q AdminQuery) ([]ManagedSource, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := []ManagedSource{}
	for _, source := range r.mediaSources {
		owner := r.usersByID[source.UserID].User
		status := "enabled"
		if source.Disabled {
			status = "disabled"
		}
		if (q.Kind == "" || q.Kind == source.Type) && (q.OwnerID == "" || q.OwnerID == source.UserID) && (q.Status == "" || q.Status == status) && adminSearch(source.Name+" "+source.Type+" "+owner.Email, q.Search) {
			items = append(items, managedSource(source, owner))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt == items[j].CreatedAt {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})
	start, end := adminBounds(len(items), q)
	return items[start:end], len(items), nil
}
func (r *MemoryRepository) AdminMutateSource(_ context.Context, id, actor string, change AdminSourceChange) (ManagedSource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !permissionGranted(r.usersByID[actor].User, "source.manage") {
		return ManagedSource{}, ErrForbidden
	}
	source, ok := r.mediaSources[id]
	if !ok {
		return ManagedSource{}, ErrNotFound
	}
	if managedSource(source, User{}).AdminVersion != change.Version {
		return ManagedSource{}, ErrAdminConflict
	}
	switch change.Action {
	case "rename":
		source.Name = change.Name
	case "enable":
		source.Disabled = false
	case "disable":
		source.Disabled = true
	case "delete":
		delete(r.mediaSources, id)
	default:
		return ManagedSource{}, ErrForbidden
	}
	source.AdminVersion = change.Version + 1
	source.UpdatedAt = time.Now().UnixMilli()
	if change.Action != "delete" {
		r.mediaSources[id] = source
	}
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "source." + change.Action, TargetType: "source", TargetID: id, Metadata: map[string]any{"reason": change.Reason}})
	return managedSource(source, r.usersByID[source.UserID].User), nil
}
func (r *MemoryRepository) GetAdminDocument(_ context.Context, key string) (AdminDocument, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d := r.adminDocuments[key]
	d.Key = key
	d.Data = append(json.RawMessage(nil), d.Data...)
	return d, nil
}
func (r *MemoryRepository) PutAdminDocument(_ context.Context, d AdminDocument, actor string) (AdminDocument, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !permissionGranted(r.usersByID[actor].User, adminDocumentPermission(d.Key)) {
		return AdminDocument{}, ErrForbidden
	}
	if r.adminDocuments[d.Key].Version != d.Version {
		return AdminDocument{}, ErrAdminConflict
	}
	d.Version++
	d.Data = append(json.RawMessage(nil), d.Data...)
	if r.adminDocuments == nil {
		r.adminDocuments = map[string]AdminDocument{}
	}
	r.adminDocuments[d.Key] = d
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "settings." + d.Key + ".update", TargetType: "settings", TargetID: d.Key})
	d.Data = append(json.RawMessage(nil), d.Data...)
	return d, nil
}
