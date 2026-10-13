package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

func (r *MemoryRepository) insertApprovalLocked(kind, userID, code string) ApprovalRequest {
	for _, item := range r.approvals {
		if item.Kind == kind && item.UserID == userID && item.RoomCode == code && item.Status == "pending" {
			return item
		}
	}
	if r.approvals == nil {
		r.approvals = map[string]ApprovalRequest{}
	}
	item := ApprovalRequest{ID: mustRandomString(16), Kind: kind, UserID: userID, RoomCode: code, Status: "pending", Version: 1, CreatedAt: time.Now().UnixMilli()}
	r.approvals[item.ID] = item
	return item
}
func (r *MemoryRepository) RegisterAccount(_ context.Context, account AccountRecord, review bool, inviteHashes ...string) (ApprovalRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.usersByMail[account.Email]; ok {
		return ApprovalRequest{}, ErrEmailExists
	}
	policy := defaultSitePolicy()
	if raw := r.adminDocuments["site_policy"].Data; len(raw) > 0 {
		if err := json.Unmarshal(raw, &policy); err != nil {
			return ApprovalRequest{}, err
		}
	}
	if !policy.RegistrationEnabled {
		return ApprovalRequest{}, ErrForbidden
	}
	if policy.RegistrationRequiresEmailCode && !account.EmailVerified {
		return ApprovalRequest{}, ErrRegistrationEmailVerification
	}
	hash := ""
	var invite RegistrationInvite
	if policy.RegistrationRequiresInvite {
		if len(inviteHashes) > 0 {
			hash = inviteHashes[0]
		}
		var exists bool
		invite, exists = r.registrationInvites[hash]
		if !exists || inviteStatus(invite, time.Now().UnixMilli()) != "active" {
			return ApprovalRequest{}, ErrRegistrationInvite
		}
	}
	account.AccountStatus = "active"
	account.AdminVersion = 1
	if review {
		account.AccountStatus = "pending"
	}
	r.usersByID[account.ID] = account
	r.usersByMail[account.Email] = account.ID
	if policy.RegistrationRequiresInvite {
		invite.UsedBy = account.ID
		invite.UsedAt = time.Now().UnixMilli()
		r.registrationInvites[hash] = invite
		r.appendAuditLocked(AuditEvent{ActorID: account.ID, Action: "registration_invites.use", TargetType: "registration_invite", TargetID: invite.ID})
	}
	if review {
		return r.insertApprovalLocked("registration", account.ID, ""), nil
	}
	return ApprovalRequest{}, nil
}
func (r *MemoryRepository) CreateRoomWithApproval(_ context.Context, room Room, f RoomFeatures, review bool) (ApprovalRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rooms[room.Code]; ok {
		return ApprovalRequest{}, ErrRoomConflict
	}
	owner, exists := r.usersByID[room.OwnerID]
	if !exists || !accountActive(owner.User) {
		return ApprovalRequest{}, ErrForbidden
	}
	if r.roomFeatures == nil {
		r.roomFeatures = map[string]RoomFeatures{}
	}
	room.Members = append([]Member(nil), room.Members...)
	r.rooms[room.Code] = room
	r.roomFeatures[room.Code] = cloneRoomFeatures(f)
	if review {
		return r.insertApprovalLocked("room_creation", room.OwnerID, room.Code), nil
	}
	return ApprovalRequest{}, nil
}
func (r *MemoryRepository) RequestRoomApproval(_ context.Context, code, userID string) (ApprovalRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[code]
	if !ok {
		return ApprovalRequest{}, ErrNotFound
	}
	user, ok := r.usersByID[userID]
	if !ok || !accountActive(user.User) || strings.HasPrefix(userID, "guest-") {
		return ApprovalRequest{}, ErrForbidden
	}
	f := r.roomFeatures[code]
	if room.Closed || room.ExpiresAt <= time.Now().UnixMilli() || f.blocked(userID) {
		return ApprovalRequest{}, ErrForbidden
	}
	if !f.RequireApproval {
		return ApprovalRequest{}, ErrAdminConflict
	}
	return r.insertApprovalLocked("room_join", userID, code), nil
}
func (r *MemoryRepository) FindApprovals(_ context.Context, actor, scope, code string, q AdminQuery) ([]ApprovalRequest, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	account, exists := r.usersByID[actor]
	a := account.User
	if !exists || (scope != "mine" && scope != "admin" && scope != "room") || !accountActive(a) || (scope == "admin" && !permissionGranted(a, "review.manage")) || (scope == "room" && r.rooms[code].OwnerID != actor && !permissionGranted(a, "review.manage")) {
		return nil, 0, ErrForbidden
	}
	items := []ApprovalRequest{}
	for _, item := range r.approvals {
		if scope == "mine" && item.UserID != actor {
			continue
		}
		if scope == "room" && (item.Kind != "room_join" || item.RoomCode != code) {
			continue
		}
		if (q.Status != "" && q.Status != item.Status) || (q.Kind != "" && q.Kind != item.Kind) {
			continue
		}
		user := r.usersByID[item.UserID]
		item.DisplayName = user.DisplayName
		if scope == "admin" || scope == "mine" {
			item.Email = user.Email
		}
		item.RoomName = r.rooms[item.RoomCode].Name
		if !adminSearch(item.DisplayName+" "+item.Email+" "+item.RoomCode+" "+item.RoomName, q.Search) {
			continue
		}
		items = append(items, item)
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
func (r *MemoryRepository) DecideApproval(_ context.Context, id, actor string, version int64, approve bool, reason string) (ApprovalRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.approvals[id]
	if !ok {
		return item, ErrNotFound
	}
	room := r.rooms[item.RoomCode]
	a, exists := r.usersByID[actor]
	if !exists || !approvalPermission(a.User, item, room) {
		return item, ErrForbidden
	}
	if item.Status != "pending" || item.Version != version {
		return item, ErrAdminConflict
	}
	user, ok := r.usersByID[item.UserID]
	if !ok {
		return item, ErrNotFound
	}
	now := time.Now().UnixMilli()
	switch item.Kind {
	case "registration":
		if user.AccountStatus != "pending" {
			if approve {
				return item, ErrAdminConflict
			}
			break
		}
		user.AccountStatus = "rejected"
		if approve {
			user.AccountStatus = "active"
		}
		user.SessionVersion++
		user.AdminVersion++
		r.usersByID[user.ID] = user
	case "room_creation":
		f := r.roomFeatures[item.RoomCode]
		if f.AdminState != "pending" {
			if approve {
				return item, ErrAdminConflict
			}
			break
		}
		if approve && (!accountActive(user.User) || room.ExpiresAt <= now) {
			return item, ErrForbidden
		}
		f.AdminState = "rejected"
		room.Closed = true
		if approve {
			f.AdminState = "active"
			room.Closed = false
		}
		f.Version++
		room.AdminEpoch = f.Version
		r.rooms[room.Code] = room
		r.roomFeatures[room.Code] = f
	case "room_join":
		if approve {
			f := r.roomFeatures[item.RoomCode]
			if room.Closed || room.ExpiresAt <= now || f.blocked(user.ID) || !accountActive(user.User) {
				return item, ErrForbidden
			}
			exists := false
			for _, m := range room.Members {
				if m.UserID == user.ID {
					exists = true
				}
			}
			if !exists {
				if len(room.Members) >= room.MaxMembers {
					return item, ErrRoomFull
				}
				room.Members = append(append([]Member(nil), room.Members...), Member{UserID: user.ID, DisplayName: user.DisplayName, JoinedAt: now})
			}
			f.Version++
			room.AdminEpoch = f.Version
			r.rooms[room.Code] = room
			r.roomFeatures[room.Code] = f
		}
	}
	item.Status = "rejected"
	if approve {
		item.Status = "approved"
	}
	item.Version++
	item.Reason = reason
	item.ReviewedBy = actor
	item.ReviewedAt = now
	r.approvals[id] = item
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "approval." + item.Status, TargetType: item.Kind, TargetID: item.ID, Metadata: map[string]any{"reason": reason}})
	return item, nil
}
