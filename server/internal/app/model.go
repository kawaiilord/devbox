package app

import (
	"encoding/json"
	"time"
)

type User struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	Email          string `json:"email,omitempty"`
	EmailVerified  bool   `json:"email_verified"`
	SessionVersion int64  `json:"-"`
	IsAdmin        bool   `json:"is_admin"`
}

type DeviceInfo struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Platform string `json:"platform"`
}

type UserDevice struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Platform  string `json:"platform"`
	Current   bool   `json:"current"`
	LastSeen  int64  `json:"last_seen"`
	CreatedAt int64  `json:"created_at"`
}

type MediaSource struct {
	ID                    string `json:"id"`
	UserID                string `json:"-"`
	Type                  string `json:"type"`
	Name                  string `json:"name"`
	BaseURL               string `json:"base_url"`
	CredentialsCiphertext string `json:"-"`
	CreatedAt             int64  `json:"created_at"`
	UpdatedAt             int64  `json:"updated_at"`
}

type MediaFile struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	IsDirectory bool   `json:"is_directory"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type,omitempty"`
	ModifiedAt  string `json:"modified_at,omitempty"`
}

type MediaTicket struct {
	UserID         string `json:"user_id"`
	SourceID       string `json:"source_id"`
	Path           string `json:"path"`
	Kind           string `json:"kind,omitempty"`
	ItemID         string `json:"item_id,omitempty"`
	MediaSourceID  string `json:"media_source_id,omitempty"`
	Container      string `json:"container,omitempty"`
	PlaySessionID  string `json:"play_session_id,omitempty"`
	SubtitleIndex  int    `json:"subtitle_index,omitempty"`
	SubtitleFormat string `json:"subtitle_format,omitempty"`
}

type Favorite struct {
	ID          int64  `json:"id"`
	UserID      string `json:"-"`
	SourceID    string `json:"source_id"`
	SourceType  string `json:"source_type"`
	SourceName  string `json:"source_name"`
	MediaPath   string `json:"media_path"`
	Title       string `json:"title"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size"`
	UpdatedAt   int64  `json:"updated_at"`
}

type WatchRecord struct {
	ID             int64   `json:"id"`
	UserID         string  `json:"-"`
	MediaKey       string  `json:"-"`
	SourceID       string  `json:"source_id,omitempty"`
	MediaPath      string  `json:"media_path,omitempty"`
	Title          string  `json:"title"`
	Position       float64 `json:"position_seconds"`
	Duration       float64 `json:"duration_seconds"`
	Episode        int     `json:"episode"`
	Completed      bool    `json:"completed"`
	CompanionCount int     `json:"companion_count"`
	RoomCode       string  `json:"room_code,omitempty"`
	Resumable      bool    `json:"resumable"`
	WatchedAt      int64   `json:"watched_at"`
}

type WatchActivity struct {
	Title          string `json:"title"`
	Episode        int    `json:"episode"`
	Completed      bool   `json:"completed"`
	CompanionCount int    `json:"companion_count"`
	WatchedAt      int64  `json:"watched_at"`
}

type DanmakuMessage struct {
	ID          int64   `json:"id"`
	Fingerprint string  `json:"-"`
	UserID      string  `json:"user_id"`
	DisplayName string  `json:"display_name"`
	Body        string  `json:"body"`
	Position    float64 `json:"position_seconds"`
	Color       int     `json:"color"`
	Mode        string  `json:"mode"`
	CreatedAt   int64   `json:"created_at"`
}

type MetadataResult struct {
	ID          int64   `json:"id"`
	MediaType   string  `json:"media_type"`
	Title       string  `json:"title"`
	Original    string  `json:"original_title,omitempty"`
	Overview    string  `json:"overview,omitempty"`
	ReleaseDate string  `json:"release_date,omitempty"`
	PosterURL   string  `json:"poster_url,omitempty"`
	Rating      float64 `json:"rating"`
}

type ChatMessage struct {
	ID          int64  `json:"id"`
	RoomCode    string `json:"room_code"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Body        string `json:"body"`
	CreatedAt   int64  `json:"created_at"`
}

type PrivacySettings struct {
	AllowRoomChat     bool `json:"allow_room_chat"`
	AllowProfileFind  bool `json:"allow_profile_find"`
	ShowWatchActivity bool `json:"show_watch_activity"`
}

type Report struct {
	ID         int64  `json:"id"`
	ReporterID string `json:"reporter_id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Reason     string `json:"reason"`
	Details    string `json:"details"`
	Status     string `json:"status"`
	Resolution string `json:"resolution,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	ReviewedBy string `json:"reviewed_by,omitempty"`
	ReviewedAt int64  `json:"reviewed_at,omitempty"`
}

type AuditEvent struct {
	ID           int64          `json:"id"`
	ActorID      string         `json:"actor_id"`
	Action       string         `json:"action"`
	TargetType   string         `json:"target_type"`
	TargetID     string         `json:"target_id"`
	Metadata     map[string]any `json:"metadata"`
	PreviousHash string         `json:"previous_hash"`
	EntryHash    string         `json:"entry_hash"`
	CreatedAt    int64          `json:"created_at"`
}

type Session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	User         User   `json:"user"`
}

type Member struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	JoinedAt    int64  `json:"joined_at"`
}

type Playback struct {
	Position      float64 `json:"position"`
	Playing       bool    `json:"playing"`
	Speed         float64 `json:"speed"`
	Episode       int     `json:"episode"`
	PositionTS    int64   `json:"position_ts"`
	SourceVersion int64   `json:"source_version"`
}

type Room struct {
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	OwnerID       string   `json:"owner_id"`
	SourceURL     string   `json:"source_url"`
	MediaSourceID string   `json:"media_source_id,omitempty"`
	MediaPath     string   `json:"media_path,omitempty"`
	MaxMembers    int      `json:"max_members"`
	CreatedAt     int64    `json:"created_at"`
	ExpiresAt     int64    `json:"expires_at"`
	Members       []Member `json:"members"`
	Playback      Playback `json:"playback"`
	Closed        bool     `json:"closed"`
}

type Envelope struct {
	Type    string          `json:"type"`
	Room    string          `json:"room"`
	Seq     int64           `json:"seq"`
	TS      int64           `json:"ts"`
	From    string          `json:"from,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

type Control struct {
	Action   string   `json:"action"`
	Position *float64 `json:"position,omitempty"`
	Speed    *float64 `json:"speed,omitempty"`
	Episode  *int     `json:"episode,omitempty"`
}

func projected(playback Playback, now time.Time) Playback {
	if playback.Playing && playback.PositionTS > 0 {
		elapsed := float64(now.UnixMilli()-playback.PositionTS) / 1000
		if elapsed > 0 {
			playback.Position += elapsed * playback.Speed
		}
	}
	playback.PositionTS = now.UnixMilli()
	return playback
}
