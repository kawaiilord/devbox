package app

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (r *MemoryRepository) GetRuntimeConfig(_ context.Context) (RuntimeConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	config := r.runtimeConfig
	config.Features = cloneBoolMap(config.Features)
	return config, nil
}

func (r *MemoryRepository) UpdateRuntimeConfig(_ context.Context, config RuntimeConfig, actor string) (RuntimeConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	config.Features = cloneBoolMap(config.Features)
	config.UpdatedAt = time.Now().UnixMilli()
	r.runtimeConfig = config
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "config.update", TargetType: "runtime_config", TargetID: "global"})
	return config, nil
}

func (r *MemoryRepository) ListAnnouncements(_ context.Context, activeOnly bool, before int64, limit int) ([]Announcement, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now().UnixMilli()
	out := []Announcement{}
	for i := len(r.announcements) - 1; i >= 0 && len(out) < limit; i-- {
		item := r.announcements[i]
		if before != 0 && item.ID >= before {
			continue
		}
		if activeOnly && (!item.Active || (item.StartsAt != 0 && item.StartsAt > now) || (item.EndsAt != 0 && item.EndsAt <= now)) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (r *MemoryRepository) CreateAnnouncement(_ context.Context, item Announcement) (Announcement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item.ID, item.CreatedAt = r.nextAnnouncement, time.Now().UnixMilli()
	r.nextAnnouncement++
	r.announcements = append(r.announcements, item)
	r.appendAuditLocked(AuditEvent{ActorID: item.CreatedBy, Action: "announcement.create", TargetType: "announcement", TargetID: strconv.FormatInt(item.ID, 10)})
	return item, nil
}

func (r *MemoryRepository) DeleteAnnouncement(_ context.Context, id int64, actor string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, item := range r.announcements {
		if item.ID == id {
			r.announcements = append(r.announcements[:index], r.announcements[index+1:]...)
			r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "announcement.delete", TargetType: "announcement", TargetID: strconv.FormatInt(id, 10)})
			return nil
		}
	}
	return ErrNotFound
}

func (r *MemoryRepository) ListDeviceBans(_ context.Context, before int64, limit int) ([]DeviceBan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []DeviceBan{}
	for hash, device := range r.devices {
		if device.banned && (before == 0 || device.bannedAt.UnixMilli() < before) {
			out = append(out, DeviceBan{DeviceHash: hash, Reason: device.banReason, BannedAt: device.bannedAt.UnixMilli(), BannedBy: device.bannedBy})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BannedAt > out[j].BannedAt })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemoryRepository) UnbanDevice(_ context.Context, hash, actor string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	device, ok := r.devices[hash]
	if !ok || !device.banned {
		return ErrNotFound
	}
	device.banned, device.banReason = false, ""
	r.devices[hash] = device
	for _, links := range r.userDevices {
		if link, ok := links[hash]; ok && link.revokedByBan {
			link.revoked, link.revokedByBan = false, false
			links[hash] = link
		}
	}
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "device.unban", TargetType: "device", TargetID: hash})
	return nil
}

func (r *MemoryRepository) AdminDashboard(_ context.Context) (AdminDashboard, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now().UnixMilli()
	value := AdminDashboard{Users: int64(len(r.usersByID)), Rooms: int64(len(r.rooms)), Reviews: int64(len(r.reviews))}
	for _, user := range r.usersByID {
		if user.EmailVerified {
			value.VerifiedUsers++
		}
		if user.VIPExpiresAt > now {
			value.ActiveVIPUsers++
		}
	}
	for _, room := range r.rooms {
		if !room.Closed && room.ExpiresAt > now {
			value.ActiveRooms++
		}
		if len(room.Members) > 1 {
			value.TogetherWatchings++
		}
	}
	for _, couple := range r.couples {
		if couple.status == "active" {
			value.Couples++
		}
	}
	for _, report := range r.reports {
		if report.Status == "pending" {
			value.PendingReports++
		}
	}
	for _, order := range r.orders {
		if order.Status == "activated" {
			value.ActivatedOrders++
			value.RevenueMinor += order.AmountMinor
		}
	}
	return value, nil
}

func (r *MemoryRepository) SearchAdminUsers(_ context.Context, query string, before int64, limit int) ([]AdminUser, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	query = strings.ToLower(query)
	out := []AdminUser{}
	for _, user := range r.usersByID {
		created := user.CreatedAt.UnixMilli()
		if (before == 0 || created < before) && (query == "" || strings.Contains(strings.ToLower(user.Email), query) || strings.Contains(strings.ToLower(user.DisplayName), query)) {
			out = append(out, AdminUser{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, EmailVerified: user.EmailVerified, AdminRole: user.AdminRole, VIPExpiresAt: user.VIPExpiresAt, CreatedAt: created})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemoryRepository) SetAdminRole(_ context.Context, userID, role, actor string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.usersByID[userID]
	if !ok {
		return ErrNotFound
	}
	account.AdminRole, account.IsAdmin = role, role != ""
	r.usersByID[userID] = account
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "user.role.update", TargetType: "user", TargetID: userID, Metadata: map[string]any{"role": role}})
	return nil
}

func (r *MemoryRepository) AdminListOrders(_ context.Context, status string, before int64, limit int) ([]Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []Order{}
	for _, order := range r.orders {
		if (status == "" || order.Status == status) && (before == 0 || order.CreatedAt < before) {
			out = append(out, order)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemoryRepository) UpdateVIPPlan(_ context.Context, plan VIPPlan, actor string) (VIPPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.vipPlans[plan.ID]; !ok {
		return VIPPlan{}, ErrNotFound
	}
	r.vipPlans[plan.ID] = plan
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "vip_plan.update", TargetType: "vip_plan", TargetID: plan.ID})
	return plan, nil
}

func (r *MemoryRepository) GetRoomBotConfig(_ context.Context) (RoomBotConfig, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	config := r.roomBotConfig
	config.HasCredential, config.Credential = r.roomBotCiphertext != "", ""
	return config, r.roomBotCiphertext, nil
}

func (r *MemoryRepository) UpdateRoomBotConfig(_ context.Context, config RoomBotConfig, ciphertext, actor string) (RoomBotConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ciphertext != "" {
		r.roomBotCiphertext = ciphertext
	}
	config.Credential, config.HasCredential, config.UpdatedAt = "", r.roomBotCiphertext != "", time.Now().UnixMilli()
	r.roomBotConfig = config
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "room_bot.update", TargetType: "room_bot", TargetID: "singleton"})
	return config, nil
}

func cloneBoolMap(source map[string]bool) map[string]bool {
	out := make(map[string]bool, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
