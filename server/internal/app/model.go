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
	UserID   string `json:"user_id"`
	SourceID string `json:"source_id"`
	Path     string `json:"path"`
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
