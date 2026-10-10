package app

import (
	"encoding/json"
	"errors"
	"strings"
)

var ErrRoomConflict = errors.New("房间已被其他人更新，请刷新后重试")

type RoomPermissions struct {
	Playback bool `json:"playback"`
	Playlist bool `json:"playlist"`
	Chat     bool `json:"chat"`
	Danmaku  bool `json:"danmaku"`
	Voice    bool `json:"voice"`
}

type PlaylistSource struct {
	IsLive        bool   `json:"is_live,omitempty"`
	ID            string `json:"id"`
	Label         string `json:"label"`
	OwnerID       string `json:"owner_id,omitempty"`
	MediaSourceID string `json:"media_source_id,omitempty"`
	MediaPath     string `json:"media_path,omitempty"`
	URL           string `json:"url,omitempty"`
	VariantID     string `json:"variant_id,omitempty"`
}

type PlaylistEntry struct {
	Episode          int              `json:"episode"`
	ID               string           `json:"id"`
	Title            string           `json:"title"`
	Sources          []PlaylistSource `json:"sources"`
	SelectedSourceID string           `json:"selected_source_id"`
}

type MediaVariant struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Height   int    `json:"height,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

// This is the private persistence record, never a response payload.
type RoomFeatures struct {
	GuestExpiries map[string]int64           `json:"guest_expiries,omitempty"`
	Version       int64                      `json:"version"`
	Visibility    string                     `json:"visibility"`
	Description   string                     `json:"description"`
	Category      string                     `json:"category"`
	Tags          []string                   `json:"tags"`
	AllowGuests   bool                       `json:"allow_guests"`
	PasswordHash  string                     `json:"password_hash,omitempty"`
	Roles         map[string]string          `json:"roles"`
	Permissions   map[string]RoomPermissions `json:"permissions"`
	BannedIDs     []string                   `json:"banned_ids"`
	Playlist      []PlaylistEntry            `json:"playlist"`
	ActiveItemID  string                     `json:"active_item_id"`
	AutoNext      bool                       `json:"auto_next"`
}

type RoomFeaturesView struct {
	Version           int64                      `json:"version"`
	Visibility        string                     `json:"visibility"`
	Description       string                     `json:"description"`
	Category          string                     `json:"category"`
	Tags              []string                   `json:"tags"`
	AllowGuests       bool                       `json:"allow_guests"`
	PasswordProtected bool                       `json:"password_protected"`
	Permissions       RoomPermissions            `json:"permissions"`
	MemberPermissions map[string]RoomPermissions `json:"member_permissions,omitempty"`
	BannedIDs         []string                   `json:"banned_ids,omitempty"`
	Playlist          []PlaylistEntry            `json:"playlist"`
	ActiveItemID      string                     `json:"active_item_id"`
	AutoNext          bool                       `json:"auto_next"`
}

type RoomMediaSelection struct {
	MediaSourceID string
	MediaPath     string
	SourceURL     string
	Playback      Playback
}

type PublicRoom struct {
	Code              string   `json:"code"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Category          string   `json:"category"`
	Tags              []string `json:"tags"`
	AllowGuests       bool     `json:"allow_guests"`
	PasswordProtected bool     `json:"password_protected"`
	Members           int      `json:"members"`
	MaxMembers        int      `json:"max_members"`
	ExpiresAt         int64    `json:"expires_at"`
}

type RoomDiscoveryQuery struct {
	Search, Category, Tag string
	Offset, Limit         int
}

func defaultRoomFeatures() RoomFeatures {
	return RoomFeatures{Visibility: "private", Tags: []string{}, Roles: map[string]string{}, Permissions: map[string]RoomPermissions{}, BannedIDs: []string{}, Playlist: []PlaylistEntry{}, AutoNext: true}
}

func cloneRoomFeatures(f RoomFeatures) RoomFeatures {
	encoded, _ := json.Marshal(f)
	copy := defaultRoomFeatures()
	_ = json.Unmarshal(encoded, &copy)
	return copy
}

func (f RoomFeatures) role(room Room, userID string) string {
	if room.OwnerID == userID {
		return "owner"
	}
	if role := f.Roles[userID]; role != "" {
		return role
	}
	return "member"
}

func (f RoomFeatures) blocked(userID string) bool {
	for _, id := range f.BannedIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func (f RoomFeatures) grants(room Room, userID string) RoomPermissions {
	if f.blocked(userID) {
		return RoomPermissions{}
	}
	role := f.role(room, userID)
	if role == "owner" {
		return RoomPermissions{true, true, true, true, true}
	}
	if role == "guest" && !f.AllowGuests {
		return RoomPermissions{}
	}
	if override, ok := f.Permissions[userID]; ok {
		return override
	}
	if role == "moderator" {
		return RoomPermissions{true, true, true, true, true}
	}
	if role == "guest" {
		return RoomPermissions{}
	}
	return RoomPermissions{Chat: true, Danmaku: true, Voice: true}
}

func (f RoomFeatures) view(room Room, viewer string) RoomFeaturesView {
	v := RoomFeaturesView{Version: f.Version, Visibility: f.Visibility, Description: f.Description, Category: f.Category, Tags: f.Tags,
		AllowGuests: f.AllowGuests, PasswordProtected: f.PasswordHash != "", Permissions: f.grants(room, viewer), Playlist: f.Playlist, ActiveItemID: f.ActiveItemID, AutoNext: f.AutoNext}
	if viewer == room.OwnerID {
		v.BannedIDs = f.BannedIDs
		v.MemberPermissions = map[string]RoomPermissions{}
		for _, member := range room.Members {
			v.MemberPermissions[member.UserID] = f.grants(room, member.UserID)
		}
	}
	return v
}

func (f RoomFeatures) activeSource() (PlaylistSource, int, bool) {
	for i, item := range f.Playlist {
		if item.ID != f.ActiveItemID {
			continue
		}
		for _, source := range item.Sources {
			if source.ID == item.SelectedSourceID {
				return source, i, true
			}
		}
	}
	return PlaylistSource{}, 0, false
}

func publicRoom(room Room, f RoomFeatures) PublicRoom {
	return PublicRoom{Code: room.Code, Name: room.Name, Description: f.Description, Category: f.Category, Tags: f.Tags,
		AllowGuests: f.AllowGuests, PasswordProtected: f.PasswordHash != "", Members: len(room.Members), MaxMembers: room.MaxMembers, ExpiresAt: room.ExpiresAt}
}

func roomDiscoveryMatches(room Room, f RoomFeatures, q RoomDiscoveryQuery) bool {
	if f.Visibility != "public" || room.Closed {
		return false
	}
	if q.Category != "" && f.Category != q.Category {
		return false
	}
	if q.Search != "" && !strings.Contains(strings.ToLower(room.Name+" "+f.Description), strings.ToLower(q.Search)) {
		return false
	}
	if q.Tag != "" {
		for _, tag := range f.Tags {
			if tag == q.Tag {
				return true
			}
		}
		return false
	}
	return true
}
