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
	AdminRole      string `json:"admin_role,omitempty"`
	Signature      string `json:"signature,omitempty"`
	VIPExpiresAt   int64  `json:"vip_expires_at,omitempty"`
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
	AllowPrivateChat  bool `json:"allow_private_chat"`
	AllowProfileFind  bool `json:"allow_profile_find"`
	ShowWatchActivity bool `json:"show_watch_activity"`
}

type SocialProfile struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	Signature      string `json:"signature,omitempty"`
	Following      bool   `json:"following"`
	FollowsViewer  bool   `json:"follows_viewer"`
	FollowerCount  int    `json:"follower_count"`
	FollowingCount int    `json:"following_count"`
}

type Conversation struct {
	ID            int64         `json:"id"`
	Peer          SocialProfile `json:"peer"`
	LastMessage   string        `json:"last_message,omitempty"`
	LastMessageAt int64         `json:"last_message_at,omitempty"`
	UnreadCount   int           `json:"unread_count"`
}

type DirectMessage struct {
	ID             int64  `json:"id"`
	ConversationID int64  `json:"conversation_id"`
	SenderID       string `json:"sender_id"`
	Body           string `json:"body"`
	CreatedAt      int64  `json:"created_at"`
}

type Couple struct {
	ID               int64         `json:"id"`
	Partner          SocialProfile `json:"partner"`
	Status           string        `json:"status"`
	BoundAt          int64         `json:"bound_at"`
	SeparatedAt      int64         `json:"separated_at,omitempty"`
	CoolingPeriodEnd int64         `json:"cooling_period_end,omitempty"`
}

type CoupleRequest struct {
	ID          int64         `json:"id"`
	Requester   SocialProfile `json:"requester"`
	RecipientID string        `json:"recipient_id,omitempty"`
	Status      string        `json:"status"`
	CreatedAt   int64         `json:"created_at"`
}

type CoupleMoment struct {
	ID        int64         `json:"id"`
	Author    SocialProfile `json:"author"`
	Body      string        `json:"body"`
	CreatedAt int64         `json:"created_at"`
}

type CoupleEvent struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	CreatedAt int64  `json:"created_at"`
}

type Review struct {
	ID         int64         `json:"id"`
	UserID     string        `json:"-"`
	Author     SocialProfile `json:"author"`
	TargetType string        `json:"target_type"`
	TargetID   string        `json:"target_id"`
	Title      string        `json:"title"`
	Rating     int           `json:"rating"`
	Content    string        `json:"content"`
	ImageKeys  []string      `json:"-"`
	ImageURLs  []string      `json:"image_urls"`
	CreatedAt  int64         `json:"created_at"`
	UpdatedAt  int64         `json:"updated_at"`
}
type ReviewComment struct {
	ID        int64         `json:"id"`
	ReviewID  int64         `json:"review_id"`
	UserID    string        `json:"-"`
	Author    SocialProfile `json:"author"`
	Body      string        `json:"body"`
	CreatedAt int64         `json:"created_at"`
}
type ObjectUpload struct {
	ObjectKey string `json:"object_key"`
	UploadURL string `json:"upload_url"`
	ExpiresAt int64  `json:"expires_at"`
}

type VIPBenefit struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

type VIPPlan struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	PriceMinor         int64  `json:"price_minor"`
	OriginalPriceMinor int64  `json:"original_price_minor,omitempty"`
	DurationDays       int    `json:"duration_days"`
	Lifetime           bool   `json:"lifetime"`
	Popular            bool   `json:"popular"`
	Enabled            bool   `json:"enabled"`
}

type VIPInfo struct {
	Announcement  string       `json:"announcement"`
	PaymentMethod string       `json:"payment_method"`
	Benefits      []VIPBenefit `json:"benefits"`
	Plans         []VIPPlan    `json:"plans"`
}

type Order struct {
	OrderNo     string `json:"order_no"`
	UserID      string `json:"-"`
	PlanID      string `json:"plan_id"`
	PlanTitle   string `json:"plan_title"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	PayChannel  string `json:"pay_channel,omitempty"`
	PayTradeNo  string `json:"pay_trade_no,omitempty"`
	CheckoutURL string `json:"checkout_url,omitempty"`
	QRExpiresAt int64  `json:"qr_expires_at"`
	CreatedAt   int64  `json:"created_at"`
	PaidAt      int64  `json:"paid_at,omitempty"`
	ActivatedAt int64  `json:"activated_at,omitempty"`
}

type PaymentIntent struct {
	CheckoutURL string `json:"checkout_url"`
	ExpiresAt   int64  `json:"expires_at"`
}

type ActivationCode struct {
	ID           int64  `json:"id"`
	CodeHash     string `json:"-"`
	BatchID      string `json:"batch_id"`
	PlanID       string `json:"plan_id,omitempty"`
	DurationDays int    `json:"duration_days"`
	UsedBy       string `json:"used_by,omitempty"`
	UsedAt       int64  `json:"used_at,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

type IssuedActivationCode struct {
	Code         string `json:"code"`
	BatchID      string `json:"batch_id"`
	PlanID       string `json:"plan_id,omitempty"`
	DurationDays int    `json:"duration_days"`
}

type CheckIn struct {
	Date   string `json:"date"`
	Points int    `json:"points"`
}

type CheckInStatus struct {
	AvailablePoints  int64     `json:"available_points"`
	TotalPoints      int64     `json:"total_points"`
	UsedPoints       int64     `json:"used_points"`
	CheckedInToday   bool      `json:"checked_in_today"`
	PointsPerCheckIn int       `json:"points_per_check_in"`
	PointsPerVIPDay  int       `json:"points_per_vip_day"`
	TotalCheckIns    int       `json:"total_check_ins"`
	ConsecutiveDays  int       `json:"consecutive_days"`
	RecentCheckIns   []CheckIn `json:"recent_check_ins"`
}

type PointsTransaction struct {
	ID           int64  `json:"id"`
	Change       int64  `json:"change"`
	BalanceAfter int64  `json:"balance_after"`
	Type         string `json:"type"`
	RefID        string `json:"ref_id,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

type PointsLeaderboardEntry struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Points      int64  `json:"points"`
	CheckIns    int    `json:"check_ins"`
}

type RuntimeConfig struct {
	Maintenance MaintenanceConfig `json:"maintenance"`
	Features    map[string]bool   `json:"features"`
	Branding    BrandingConfig    `json:"branding"`
	UpdatedAt   int64             `json:"updated_at"`
}

type MaintenanceConfig struct {
	Enabled bool   `json:"enabled"`
	Message string `json:"message"`
}

type BrandingConfig struct {
	GlobalAnnouncement    string `json:"global_announcement"`
	StartupAnnouncementID int64  `json:"startup_announcement_id"`
}

type Announcement struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Kind      string `json:"kind"`
	Active    bool   `json:"active"`
	StartsAt  int64  `json:"starts_at,omitempty"`
	EndsAt    int64  `json:"ends_at,omitempty"`
	CreatedBy string `json:"created_by,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

type DeviceBan struct {
	DeviceHash string `json:"device_hash"`
	Reason     string `json:"reason"`
	BannedAt   int64  `json:"banned_at"`
	BannedBy   string `json:"banned_by"`
}

type AdminDashboard struct {
	Users                      int64 `json:"users"`
	VerifiedUsers              int64 `json:"verified_users"`
	ActiveVIPUsers             int64 `json:"active_vip_users"`
	Rooms                      int64 `json:"rooms"`
	ActiveRooms                int64 `json:"active_rooms"`
	Couples                    int64 `json:"couples"`
	PendingReports             int64 `json:"pending_reports"`
	ActivatedOrders            int64 `json:"activated_orders"`
	RevenueMinor               int64 `json:"revenue_minor"`
	Reviews                    int64 `json:"reviews"`
	TogetherWatchings          int64 `json:"together_watchings"`
	PendingDeletions           int64 `json:"pending_deletions"`
	OpenCopyrightComplaints    int64 `json:"open_copyright_complaints"`
	OverdueCopyrightComplaints int64 `json:"overdue_copyright_complaints"`
}

type RoomBotConfig struct {
	Enabled         bool   `json:"enabled"`
	DisplayName     string `json:"display_name"`
	SummonPolicy    string `json:"summon_policy"`
	ReplyPolicy     string `json:"reply_policy"`
	ProviderBaseURL string `json:"provider_base_url"`
	Model           string `json:"model"`
	HasCredential   bool   `json:"has_credential"`
	Credential      string `json:"credential,omitempty"`
	UpdatedAt       int64  `json:"updated_at"`
}

type AdminUser struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name"`
	EmailVerified bool   `json:"email_verified"`
	AdminRole     string `json:"admin_role,omitempty"`
	VIPExpiresAt  int64  `json:"vip_expires_at,omitempty"`
	CreatedAt     int64  `json:"created_at"`
}

type AccountDeletionRequest struct {
	ID          int64  `json:"id"`
	UserID      string `json:"user_id,omitempty"`
	Reason      string `json:"reason"`
	Status      string `json:"status"`
	RequestedAt int64  `json:"requested_at"`
	ReviewedBy  string `json:"reviewed_by,omitempty"`
	ReviewedAt  int64  `json:"reviewed_at,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
	ExecutedAt  int64  `json:"executed_at,omitempty"`
}

type CopyrightComplaint struct {
	ID                int64    `json:"id"`
	ClaimantUserID    string   `json:"claimant_user_id,omitempty"`
	ClaimantName      string   `json:"claimant_name"`
	ClaimantEmail     string   `json:"claimant_email"`
	RightsBasis       string   `json:"rights_basis"`
	InfringementURL   string   `json:"infringement_url"`
	RoomCode          string   `json:"room_code,omitempty"`
	Evidence          []string `json:"evidence"`
	StatementAccurate bool     `json:"statement_accurate"`
	SignatureName     string   `json:"signature_name"`
	Status            string   `json:"status"`
	SubmittedAt       int64    `json:"submitted_at"`
	DueAt             int64    `json:"due_at"`
	ReviewedBy        string   `json:"reviewed_by,omitempty"`
	ReviewedAt        int64    `json:"reviewed_at,omitempty"`
	Resolution        string   `json:"resolution,omitempty"`
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
