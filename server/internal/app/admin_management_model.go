package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrAdminConflict = errors.New("记录已被更新，请刷新后重试")

type AdminManagementRepository interface {
	AdminFindUsers(context.Context, AdminQuery) ([]ManagedUser, int, error)
	AdminCreateUser(context.Context, AccountRecord, string) (ManagedUser, error)
	AdminMutateUser(context.Context, string, string, AdminUserChange) (ManagedUser, error)
	AdminFindRooms(context.Context, AdminQuery) ([]AdminRoomRecord, int, error)
	AdminReadRoom(context.Context, string) (AdminRoomRecord, error)
	AdminSaveRoom(context.Context, AdminRoomRecord, int64, string, string) error
	AdminFindSources(context.Context, AdminQuery) ([]ManagedSource, int, error)
	AdminReadSource(context.Context, string) (ManagedSource, error)
	AdminMutateSource(context.Context, string, string, AdminSourceChange) (ManagedSource, error)
	GetAdminDocument(context.Context, string) (AdminDocument, error)
	PutAdminDocument(context.Context, AdminDocument, string) (AdminDocument, error)
}

type AdminQuery struct {
	Search, Status, OwnerID, Kind, Category string
	Offset, Limit                           int
}

type ManagedUser struct {
	AdminUser
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Version int64  `json:"version"`
}

type AdminUserChange struct {
	Version      int64  `json:"version"`
	Action       string `json:"action"`
	DisplayName  string `json:"display_name"`
	Role         string `json:"role"`
	Reason       string `json:"reason"`
	PasswordHash string `json:"-"`
}

type AdminRoomRecord struct {
	Room       Room
	Features   RoomFeatures
	OwnerName  string
	OwnerEmail string
}

type ManagedSource struct {
	MediaSource
	OwnerID    string `json:"owner_id"`
	OwnerName  string `json:"owner_name"`
	OwnerEmail string `json:"owner_email"`
}

type AdminSourceChange struct {
	Version int64  `json:"version"`
	Action  string `json:"action"`
	Name    string `json:"name"`
	Reason  string `json:"reason"`
}

type AdminDocument struct {
	Key     string          `json:"-"`
	Version int64           `json:"version"`
	Data    json.RawMessage `json:"data"`
}

type SitePolicy struct {
	RegistrationRequiresReview bool `json:"registration_requires_review"`
	RoomCreationRequiresReview bool `json:"room_creation_requires_review"`
	RegistrationEnabled        bool `json:"registration_enabled"`
	RoomCreationEnabled        bool `json:"room_creation_enabled"`
	GuestsEnabled              bool `json:"guests_enabled"`
	ChatEnabled                bool `json:"chat_enabled"`
	MaxRoomsPerUser            int  `json:"max_rooms_per_user"`
	DefaultMaxMembers          int  `json:"default_max_members"`
	MaxRoomMembers             int  `json:"max_room_members"`
	ChatHistoryLimit           int  `json:"chat_history_limit"`
}

type TaxonomyEntry struct {
	Key         string `json:"key"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	SortOrder   int    `json:"sort_order"`
	Enabled     bool   `json:"enabled"`
}

func defaultSitePolicy() SitePolicy {
	return SitePolicy{RegistrationEnabled: true, RoomCreationEnabled: true, GuestsEnabled: true,
		ChatEnabled: true, DefaultMaxMembers: 8, MaxRoomMembers: 100, ChatHistoryLimit: 50}
}

func accountActive(user User) bool { return user.AccountStatus == "" || user.AccountStatus == "active" }
func adminRole(user User) string {
	if user.AdminRole != "" {
		return user.AdminRole
	}
	if user.IsAdmin {
		return "super_admin"
	}
	return ""
}
func permissionGranted(user User, permission string) bool {
	grants := rolePermissions[adminRole(user)]
	return accountActive(user) && user.EmailVerified && (grants["*"] || grants[permission])
}
func managedUser(account AccountRecord) ManagedUser {
	status := account.AccountStatus
	if status == "" {
		status = "active"
	}
	version := account.AdminVersion
	if version == 0 {
		version = 1
	}
	return ManagedUser{AdminUser: AdminUser{ID: account.ID, Email: account.Email, DisplayName: account.DisplayName,
		EmailVerified: account.EmailVerified, AdminRole: adminRole(account.User), VIPExpiresAt: account.VIPExpiresAt,
		CreatedAt: account.CreatedAt.UnixMilli()}, Status: status, Reason: account.StatusReason, Version: version}
}
func canManageAccount(actor, target User) bool {
	if !permissionGranted(actor, "user.manage") || actor.ID == target.ID {
		return false
	}
	return adminRole(actor) == "super_admin" || adminRole(target) == ""
}
func applyAdminUserChange(actor User, account *AccountRecord, change AdminUserChange) error {
	if !canManageAccount(actor, account.User) {
		return ErrForbidden
	}
	if change.Version != managedUser(*account).Version {
		return ErrAdminConflict
	}
	if account.AccountStatus == "deleted" && change.Action != "restore" {
		return ErrAdminConflict
	}
	switch change.Action {
	case "profile":
		account.DisplayName = strings.TrimSpace(change.DisplayName)
	case "password":
		account.PasswordHash = change.PasswordHash
		account.SessionVersion++
	case "role":
		if !permissionGranted(actor, "role.manage") {
			return ErrForbidden
		}
		account.AdminRole, account.IsAdmin = change.Role, change.Role != ""
	case "ban":
		account.AccountStatus, account.StatusReason = "banned", change.Reason
		account.SessionVersion++
	case "unban":
		if account.AccountStatus != "banned" {
			return ErrAdminConflict
		}
		account.AccountStatus, account.StatusReason = "active", ""
		account.SessionVersion++
	case "delete":
		account.AccountStatus, account.StatusReason = "deleted", change.Reason
		account.SessionVersion++
	case "restore":
		if account.AccountStatus != "deleted" {
			return ErrAdminConflict
		}
		account.AccountStatus, account.StatusReason = "active", ""
		account.SessionVersion++
	default:
		return errors.New("不支持的账号操作")
	}
	account.AdminVersion = managedUser(*account).Version + 1
	return nil
}
func accountChangeRevokes(action string) bool {
	return action == "password" || action == "ban" || action == "delete" || action == "restore" || action == "unban"
}
func roomAdminStatus(record AdminRoomRecord) string {
	if record.Features.AdminState != "" && record.Features.AdminState != "active" {
		return record.Features.AdminState
	}
	if record.Room.Closed {
		return "closed"
	}
	if record.Room.ExpiresAt <= time.Now().UnixMilli() {
		return "expired"
	}
	return "active"
}
func adminDocumentPermission(key string) string {
	if key == "taxonomy" {
		return "taxonomy.manage"
	}
	return "config.write"
}
