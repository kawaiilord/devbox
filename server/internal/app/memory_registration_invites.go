package app

import (
	"context"
	"sort"
	"strings"
	"time"
)

func (r *MemoryRepository) StoreRegistrationInvites(_ context.Context, actor string, codes []RegistrationInvite) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !permissionGranted(r.usersByID[actor].User, "config.write") {
		return ErrForbidden
	}
	if r.registrationInvites == nil {
		r.registrationInvites = map[string]RegistrationInvite{}
	}
	for _, code := range codes {
		if _, ok := r.registrationInvites[code.CodeHash]; ok {
			return ErrAdminConflict
		}
	}
	for _, code := range codes {
		r.registrationInvites[code.CodeHash] = code
	}
	if len(codes) > 0 {
		r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "registration_invites.create", TargetType: "registration_invite_batch", TargetID: codes[0].BatchID, Metadata: map[string]any{"count": len(codes), "label": codes[0].Label}})
	}
	return nil
}

func (r *MemoryRepository) FindRegistrationInvites(_ context.Context, q AdminQuery) ([]RegistrationInvite, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := []RegistrationInvite{}
	now := time.Now().UnixMilli()
	for _, code := range r.registrationInvites {
		code.Status = inviteStatus(code, now)
		if q.Status != "" && q.Status != code.Status {
			continue
		}
		if q.Search != "" && !strings.Contains(strings.ToLower(code.Label+" "+code.BatchID+" "+code.Hint), strings.ToLower(q.Search)) {
			continue
		}
		items = append(items, code)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt == items[j].CreatedAt {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})
	start, end := adminBounds(len(items), q)
	return items[start:end], len(items), nil
}

func (r *MemoryRepository) RevokeRegistrationInvite(_ context.Context, actor, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !permissionGranted(r.usersByID[actor].User, "config.write") {
		return ErrForbidden
	}
	for hash, code := range r.registrationInvites {
		if code.ID == id {
			if code.UsedAt > 0 || code.RevokedAt > 0 {
				return ErrAdminConflict
			}
			code.RevokedAt = time.Now().UnixMilli()
			r.registrationInvites[hash] = code
			r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "registration_invites.revoke", TargetType: "registration_invite", TargetID: id})
			return nil
		}
	}
	return ErrNotFound
}
