package app

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrEmailExists        = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh     = errors.New("invalid or expired refresh token")
	ErrDeviceBlocked      = errors.New("device is blocked or revoked")
	ErrInvalidActionToken = errors.New("invalid or expired action token")
)

type AccountRecord struct {
	User
	PasswordHash string
	CreatedAt    time.Time
}

type RefreshIdentity struct {
	Account    AccountRecord
	DeviceHash string
}

type ActionTokenRecord struct {
	Hash      string
	UserID    string
	Purpose   string
	ExpiresAt time.Time
}

type Repository interface {
	CreateUser(context.Context, AccountRecord) error
	UserByEmail(context.Context, string) (AccountRecord, error)
	UserByID(context.Context, string) (AccountRecord, error)
	StoreRefreshToken(context.Context, string, string, string, time.Time) error
	RotateRefreshToken(context.Context, string, string, string, time.Time) (RefreshIdentity, error)
	RevokeRefreshToken(context.Context, string) error
	RevokeUserRefreshTokens(context.Context, string) error
	BindDevice(context.Context, string, string, DeviceInfo) error
	ValidateDevice(context.Context, string, string) error
	UserDevices(context.Context, string) ([]UserDevice, error)
	RevokeUserDevice(context.Context, string, string) error
	StoreActionToken(context.Context, ActionTokenRecord) error
	ConsumeActionToken(context.Context, string, string) (AccountRecord, error)
	VerifyEmail(context.Context, string) error
	UpdatePassword(context.Context, string, string) error
	CreateMediaSource(context.Context, MediaSource) error
	ListMediaSources(context.Context, string) ([]MediaSource, error)
	GetMediaSource(context.Context, string, string) (MediaSource, error)
	DeleteMediaSource(context.Context, string, string) error
	UpsertFavorite(context.Context, Favorite) (Favorite, error)
	ListFavorites(context.Context, string, int64, int) ([]Favorite, error)
	DeleteFavorite(context.Context, string, int64) error
	UpsertWatchRecord(context.Context, WatchRecord) (WatchRecord, error)
	ListWatchRecords(context.Context, string, int64, int) ([]WatchRecord, error)
	DeleteWatchRecord(context.Context, string, int64) error
	AddDanmaku(context.Context, DanmakuMessage) (DanmakuMessage, error)
	ListDanmaku(context.Context, string, string, float64, float64, int) ([]DanmakuMessage, error)
	AddRoomMessage(context.Context, ChatMessage) (ChatMessage, error)
	ListRoomMessages(context.Context, string, string, int64, int) ([]ChatMessage, error)
	GetPrivacy(context.Context, string) (PrivacySettings, error)
	UpdatePrivacy(context.Context, string, PrivacySettings) error
	BlockUser(context.Context, string, string) error
	UnblockUser(context.Context, string, string) error
	ListBlockedUsers(context.Context, string) ([]User, error)
	UsersBlocked(context.Context, string, string) (bool, error)
	CreateReport(context.Context, Report) (Report, error)
	ListReports(context.Context, string, int64, int) ([]Report, error)
	ResolveReport(context.Context, int64, string, string, string) (Report, error)
	CloseRoom(context.Context, string, string) error
	BanDevice(context.Context, string, string, string) error
	SetUserAdmin(context.Context, string, bool) error
	SetUserVIP(context.Context, string, int64) error
	AppendAudit(context.Context, AuditEvent) (AuditEvent, error)
	ListAudit(context.Context, int64, int) ([]AuditEvent, error)
	SearchSocialProfiles(context.Context, string, string, int) ([]SocialProfile, error)
	GetSocialProfile(context.Context, string, string) (SocialProfile, error)
	FollowUser(context.Context, string, string) error
	UnfollowUser(context.Context, string, string) error
	CreateConversation(context.Context, string, string) (Conversation, error)
	ListConversations(context.Context, string, int) ([]Conversation, error)
	ListDirectMessages(context.Context, string, int64, int64, int) ([]DirectMessage, error)
	ConversationPeer(context.Context, string, int64) (string, error)
	AddDirectMessage(context.Context, string, int64, string) (DirectMessage, string, error)
	MarkConversationRead(context.Context, string, int64, int64) error
	UnreadDirectCount(context.Context, string) (int, error)
	CreateCoupleRequest(context.Context, string, string) (CoupleRequest, error)
	ListCoupleRequests(context.Context, string) ([]CoupleRequest, error)
	RespondCoupleRequest(context.Context, string, int64, bool) (Couple, error)
	GetCouple(context.Context, string) (Couple, error)
	SeparateCouple(context.Context, string) (Couple, error)
	RestoreCouple(context.Context, string) (Couple, error)
	AddCoupleMoment(context.Context, string, string) (CoupleMoment, error)
	ListCoupleMoments(context.Context, string, int64, int) ([]CoupleMoment, error)
	ListCoupleEvents(context.Context, string, int) ([]CoupleEvent, error)
	UpsertReview(context.Context, Review) (Review, []string, error)
	ListReviews(context.Context, string, string, string, int64, int) ([]Review, error)
	DeleteReview(context.Context, string, int64) ([]string, error)
	AddReviewComment(context.Context, ReviewComment) (ReviewComment, error)
	ListReviewComments(context.Context, string, int64, int64, int) ([]ReviewComment, error)
	ListVIPPlans(context.Context, bool) ([]VIPPlan, error)
	GetVIPPlan(context.Context, string) (VIPPlan, error)
	CreateOrder(context.Context, Order) (Order, error)
	SetOrderCheckout(context.Context, string, string, int64) error
	GetOrder(context.Context, string, string) (Order, error)
	ListUserOrders(context.Context, string, int64, int) ([]Order, error)
	ActivatePaidOrder(context.Context, string, string, string, int64) (Order, bool, error)
	StoreActivationCodes(context.Context, []ActivationCode) error
	RedeemActivationCode(context.Context, string, string) (int64, error)
	DailyCheckIn(context.Context, string, string, int) (CheckInStatus, error)
	GetCheckInStatus(context.Context, string, string, int, int) (CheckInStatus, error)
	RedeemPointsForVIP(context.Context, string, int, int) (int64, error)
	ListPointsTransactions(context.Context, string, int64, int) ([]PointsTransaction, error)
	PointsLeaderboard(context.Context, string, int) ([]PointsLeaderboardEntry, error)
	SaveRoom(context.Context, Room) error
	SaveMember(context.Context, string, Member) error
	UpdatePlayback(context.Context, string, Playback) error
	LoadRooms(context.Context) ([]Room, error)
	Close()
}

type memoryRefreshToken struct {
	userID     string
	deviceHash string
	expiresAt  time.Time
	revoked    bool
}

type memoryDevice struct {
	info      DeviceInfo
	createdAt time.Time
	lastSeen  time.Time
	banned    bool
}

type memoryUserDevice struct {
	revoked  bool
	lastSeen time.Time
}

type memoryActionToken struct {
	record ActionTokenRecord
	used   bool
}

type MemoryRepository struct {
	mu                    sync.RWMutex
	usersByID             map[string]AccountRecord
	usersByMail           map[string]string
	refresh               map[string]memoryRefreshToken
	devices               map[string]memoryDevice
	userDevices           map[string]map[string]memoryUserDevice
	actionTokens          map[string]memoryActionToken
	mediaSources          map[string]MediaSource
	favorites             map[int64]Favorite
	favoriteKeys          map[string]int64
	nextFavorite          int64
	watchRecords          map[int64]WatchRecord
	watchKeys             map[string]int64
	nextWatch             int64
	danmaku               []DanmakuMessage
	nextDanmaku           int64
	messages              []ChatMessage
	nextMessageID         int64
	rooms                 map[string]Room
	privacy               map[string]PrivacySettings
	blocks                map[string]map[string]time.Time
	reports               []Report
	nextReportID          int64
	audit                 []AuditEvent
	nextAuditID           int64
	follows               map[string]map[string]time.Time
	conversations         map[int64]memoryConversation
	conversationKeys      map[string]int64
	nextConversation      int64
	directMessages        map[int64][]DirectMessage
	nextDirectMessage     int64
	conversationReads     map[int64]map[string]int64
	coupleRequests        map[int64]memoryCoupleRequest
	nextCoupleRequest     int64
	couples               map[int64]memoryCouple
	nextCouple            int64
	coupleMoments         map[int64][]memoryCoupleMoment
	nextCoupleMoment      int64
	coupleEvents          map[int64][]CoupleEvent
	nextCoupleEvent       int64
	reviews               map[int64]Review
	reviewKeys            map[string]int64
	nextReview            int64
	reviewComments        map[int64][]ReviewComment
	nextReviewComment     int64
	vipPlans              map[string]VIPPlan
	orders                map[string]Order
	activationCodes       map[string]ActivationCode
	checkIns              map[string]map[string]CheckIn
	pointsAccounts        map[string]memoryPointsAccount
	pointsTransactions    map[string][]PointsTransaction
	nextPointsTransaction int64
}

type memoryConversation struct {
	id       int64
	userLow  string
	userHigh string
	updated  time.Time
}
type memoryCoupleRequest struct {
	id                           int64
	requester, recipient, status string
	created                      time.Time
}
type memoryCouple struct {
	id                        int64
	low, high, status         string
	bound, separated, cooling time.Time
}
type memoryCoupleMoment struct {
	id           int64
	author, body string
	created      time.Time
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		usersByID:         make(map[string]AccountRecord),
		usersByMail:       make(map[string]string),
		refresh:           make(map[string]memoryRefreshToken),
		devices:           make(map[string]memoryDevice),
		userDevices:       make(map[string]map[string]memoryUserDevice),
		actionTokens:      make(map[string]memoryActionToken),
		mediaSources:      make(map[string]MediaSource),
		favorites:         make(map[int64]Favorite),
		favoriteKeys:      make(map[string]int64),
		nextFavorite:      1,
		watchRecords:      make(map[int64]WatchRecord),
		watchKeys:         make(map[string]int64),
		nextWatch:         1,
		nextDanmaku:       1,
		nextMessageID:     1,
		rooms:             make(map[string]Room),
		privacy:           make(map[string]PrivacySettings),
		blocks:            make(map[string]map[string]time.Time),
		nextReportID:      1,
		nextAuditID:       1,
		follows:           make(map[string]map[string]time.Time),
		conversations:     make(map[int64]memoryConversation),
		conversationKeys:  make(map[string]int64),
		nextConversation:  1,
		directMessages:    make(map[int64][]DirectMessage),
		nextDirectMessage: 1,
		conversationReads: make(map[int64]map[string]int64),
		coupleRequests:    make(map[int64]memoryCoupleRequest), nextCoupleRequest: 1,
		couples: make(map[int64]memoryCouple), nextCouple: 1,
		coupleMoments: make(map[int64][]memoryCoupleMoment), nextCoupleMoment: 1,
		coupleEvents: make(map[int64][]CoupleEvent), nextCoupleEvent: 1,
		reviews: make(map[int64]Review), reviewKeys: make(map[string]int64), nextReview: 1,
		reviewComments: make(map[int64][]ReviewComment), nextReviewComment: 1,
		vipPlans: map[string]VIPPlan{
			"monthly":  {ID: "monthly", Title: "月度会员", PriceMinor: 800, DurationDays: 30, Enabled: true},
			"annual":   {ID: "annual", Title: "年度会员", PriceMinor: 5800, OriginalPriceMinor: 9800, DurationDays: 365, Popular: true, Enabled: true},
			"lifetime": {ID: "lifetime", Title: "终身会员", PriceMinor: 4900, OriginalPriceMinor: 13600, Lifetime: true, Enabled: true},
		},
		orders: make(map[string]Order), activationCodes: make(map[string]ActivationCode),
		checkIns: make(map[string]map[string]CheckIn), pointsAccounts: make(map[string]memoryPointsAccount),
		pointsTransactions: make(map[string][]PointsTransaction), nextPointsTransaction: 1,
	}
}

func (r *MemoryRepository) CreateUser(_ context.Context, account AccountRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	email := strings.ToLower(account.Email)
	if _, exists := r.usersByMail[email]; exists {
		return ErrEmailExists
	}
	r.usersByID[account.ID] = account
	r.usersByMail[email] = account.ID
	return nil
}

func (r *MemoryRepository) UserByEmail(_ context.Context, email string) (AccountRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.usersByMail[strings.ToLower(email)]
	if !ok {
		return AccountRecord{}, ErrInvalidCredentials
	}
	return r.usersByID[id], nil
}

func (r *MemoryRepository) UserByID(_ context.Context, id string) (AccountRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	account, ok := r.usersByID[id]
	if !ok {
		return AccountRecord{}, ErrNotFound
	}
	return account, nil
}

func (r *MemoryRepository) StoreRefreshToken(_ context.Context, userID, deviceHash, hash string, expiresAt time.Time) error {
	r.mu.Lock()
	r.refresh[hash] = memoryRefreshToken{userID: userID, deviceHash: deviceHash, expiresAt: expiresAt}
	r.mu.Unlock()
	return nil
}

func (r *MemoryRepository) RotateRefreshToken(_ context.Context, oldHash, expectedDeviceHash, newHash string, expiresAt time.Time) (RefreshIdentity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.refresh[oldHash]
	if !ok || token.revoked || !token.expiresAt.After(time.Now()) {
		return RefreshIdentity{}, ErrInvalidRefresh
	}
	if token.deviceHash != expectedDeviceHash {
		return RefreshIdentity{}, ErrInvalidRefresh
	}
	token.revoked = true
	r.refresh[oldHash] = token
	r.refresh[newHash] = memoryRefreshToken{userID: token.userID, deviceHash: token.deviceHash, expiresAt: expiresAt}
	account, ok := r.usersByID[token.userID]
	if !ok {
		return RefreshIdentity{}, ErrInvalidRefresh
	}
	return RefreshIdentity{Account: account, DeviceHash: token.deviceHash}, nil
}

func (r *MemoryRepository) RevokeRefreshToken(_ context.Context, hash string) error {
	r.mu.Lock()
	token, ok := r.refresh[hash]
	if ok {
		token.revoked = true
		r.refresh[hash] = token
	}
	r.mu.Unlock()
	return nil
}

func (r *MemoryRepository) RevokeUserRefreshTokens(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for hash, token := range r.refresh {
		if token.userID == userID {
			token.revoked = true
			r.refresh[hash] = token
		}
	}
	return nil
}

func (r *MemoryRepository) BindDevice(_ context.Context, userID, deviceHash string, info DeviceInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	device := r.devices[deviceHash]
	if device.banned {
		return ErrDeviceBlocked
	}
	if device.createdAt.IsZero() {
		device.createdAt = now
	}
	device.info = info
	device.lastSeen = now
	r.devices[deviceHash] = device
	if r.userDevices[userID] == nil {
		r.userDevices[userID] = make(map[string]memoryUserDevice)
	}
	if existing, ok := r.userDevices[userID][deviceHash]; ok && existing.revoked {
		return ErrDeviceBlocked
	}
	r.userDevices[userID][deviceHash] = memoryUserDevice{lastSeen: now}
	return nil
}

func (r *MemoryRepository) ValidateDevice(_ context.Context, userID, deviceHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	device, ok := r.devices[deviceHash]
	link, linked := r.userDevices[userID][deviceHash]
	if !ok || !linked || device.banned || link.revoked {
		return ErrDeviceBlocked
	}
	now := time.Now()
	device.lastSeen = now
	link.lastSeen = now
	r.devices[deviceHash] = device
	r.userDevices[userID][deviceHash] = link
	return nil
}

func (r *MemoryRepository) UserDevices(_ context.Context, userID string) ([]UserDevice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]UserDevice, 0, len(r.userDevices[userID]))
	for hash, link := range r.userDevices[userID] {
		if link.revoked {
			continue
		}
		device := r.devices[hash]
		result = append(result, UserDevice{
			ID: hash, Label: device.info.Label, Platform: device.info.Platform,
			LastSeen: link.lastSeen.UnixMilli(), CreatedAt: device.createdAt.UnixMilli(),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LastSeen > result[j].LastSeen })
	return result, nil
}

func (r *MemoryRepository) RevokeUserDevice(_ context.Context, userID, deviceHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	links := r.userDevices[userID]
	link, ok := links[deviceHash]
	if !ok {
		return ErrNotFound
	}
	link.revoked = true
	links[deviceHash] = link
	for hash, token := range r.refresh {
		if token.userID == userID && token.deviceHash == deviceHash {
			token.revoked = true
			r.refresh[hash] = token
		}
	}
	return nil
}

func (r *MemoryRepository) StoreActionToken(_ context.Context, record ActionTokenRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for hash, token := range r.actionTokens {
		if token.record.UserID == record.UserID && token.record.Purpose == record.Purpose && !token.used {
			token.used = true
			r.actionTokens[hash] = token
		}
	}
	r.actionTokens[record.Hash] = memoryActionToken{record: record}
	return nil
}

func (r *MemoryRepository) ConsumeActionToken(_ context.Context, hash, purpose string) (AccountRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.actionTokens[hash]
	if !ok || token.used || token.record.Purpose != purpose || !token.record.ExpiresAt.After(time.Now()) {
		return AccountRecord{}, ErrInvalidActionToken
	}
	token.used = true
	r.actionTokens[hash] = token
	account, ok := r.usersByID[token.record.UserID]
	if !ok {
		return AccountRecord{}, ErrInvalidActionToken
	}
	return account, nil
}

func (r *MemoryRepository) VerifyEmail(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.usersByID[userID]
	if !ok {
		return ErrNotFound
	}
	account.EmailVerified = true
	r.usersByID[userID] = account
	return nil
}

func (r *MemoryRepository) UpdatePassword(_ context.Context, userID, passwordHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.usersByID[userID]
	if !ok {
		return ErrNotFound
	}
	account.PasswordHash = passwordHash
	account.SessionVersion++
	r.usersByID[userID] = account
	return nil
}

func (r *MemoryRepository) CreateMediaSource(_ context.Context, source MediaSource) error {
	r.mu.Lock()
	r.mediaSources[source.ID] = source
	r.mu.Unlock()
	return nil
}

func (r *MemoryRepository) ListMediaSources(_ context.Context, userID string) ([]MediaSource, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]MediaSource, 0)
	for _, source := range r.mediaSources {
		if source.UserID == userID {
			result = append(result, source)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt < result[j].CreatedAt })
	return result, nil
}

func (r *MemoryRepository) GetMediaSource(_ context.Context, userID, sourceID string) (MediaSource, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	source, ok := r.mediaSources[sourceID]
	if !ok || source.UserID != userID {
		return MediaSource{}, ErrNotFound
	}
	return source, nil
}

func (r *MemoryRepository) DeleteMediaSource(_ context.Context, userID, sourceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	source, ok := r.mediaSources[sourceID]
	if !ok || source.UserID != userID {
		return ErrNotFound
	}
	delete(r.mediaSources, sourceID)
	for id, favorite := range r.favorites {
		if favorite.SourceID == sourceID {
			delete(r.favoriteKeys, memoryFavoriteKey(favorite.UserID, favorite.SourceID, favorite.MediaPath))
			delete(r.favorites, id)
		}
	}
	for id, record := range r.watchRecords {
		if record.SourceID == sourceID {
			record.SourceID = ""
			record.MediaPath = ""
			record.Resumable = false
			r.watchRecords[id] = record
		}
	}
	return nil
}

func (r *MemoryRepository) UpsertFavorite(_ context.Context, favorite Favorite) (Favorite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	source, ok := r.mediaSources[favorite.SourceID]
	if !ok || source.UserID != favorite.UserID {
		return Favorite{}, ErrNotFound
	}
	key := memoryFavoriteKey(favorite.UserID, favorite.SourceID, favorite.MediaPath)
	if id, ok := r.favoriteKeys[key]; ok {
		favorite.ID = id
	} else {
		favorite.ID = r.nextFavorite
		r.nextFavorite++
		r.favoriteKeys[key] = favorite.ID
	}
	favorite.SourceType = source.Type
	favorite.SourceName = source.Name
	favorite.UpdatedAt = time.Now().UnixMilli()
	r.favorites[favorite.ID] = favorite
	return favorite, nil
}

func (r *MemoryRepository) ListFavorites(_ context.Context, userID string, before int64, limit int) ([]Favorite, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Favorite, 0)
	for _, favorite := range r.favorites {
		if favorite.UserID == userID && (before == 0 || favorite.UpdatedAt < before) {
			if source, ok := r.mediaSources[favorite.SourceID]; ok {
				favorite.SourceType = source.Type
				favorite.SourceName = source.Name
				result = append(result, favorite)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt == result[j].UpdatedAt {
			return result[i].ID > result[j].ID
		}
		return result[i].UpdatedAt > result[j].UpdatedAt
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) DeleteFavorite(_ context.Context, userID string, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	favorite, ok := r.favorites[id]
	if !ok || favorite.UserID != userID {
		return ErrNotFound
	}
	delete(r.favoriteKeys, memoryFavoriteKey(favorite.UserID, favorite.SourceID, favorite.MediaPath))
	delete(r.favorites, id)
	return nil
}

func (r *MemoryRepository) UpsertWatchRecord(_ context.Context, record WatchRecord) (WatchRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if record.SourceID != "" {
		source, ok := r.mediaSources[record.SourceID]
		if !ok || source.UserID != record.UserID {
			record.SourceID = ""
			record.MediaPath = ""
		}
	}
	key := record.UserID + "\x00" + record.MediaKey
	if id, ok := r.watchKeys[key]; ok {
		record.ID = id
		previous := r.watchRecords[id]
		if previous.CompanionCount > record.CompanionCount {
			record.CompanionCount = previous.CompanionCount
		}
		record.Completed = record.Completed || previous.Completed
	} else {
		record.ID = r.nextWatch
		r.nextWatch++
		r.watchKeys[key] = record.ID
	}
	record.WatchedAt = time.Now().UnixMilli()
	if record.SourceID != "" {
		source, ok := r.mediaSources[record.SourceID]
		record.Resumable = ok && source.UserID == record.UserID
	}
	r.watchRecords[record.ID] = record
	return record, nil
}

func (r *MemoryRepository) ListWatchRecords(_ context.Context, userID string, before int64, limit int) ([]WatchRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]WatchRecord, 0)
	for _, record := range r.watchRecords {
		if record.UserID != userID || (before != 0 && record.WatchedAt >= before) {
			continue
		}
		if record.SourceID != "" {
			source, ok := r.mediaSources[record.SourceID]
			record.Resumable = ok && source.UserID == userID
		} else {
			record.MediaPath = ""
		}
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].WatchedAt == result[j].WatchedAt {
			return result[i].ID > result[j].ID
		}
		return result[i].WatchedAt > result[j].WatchedAt
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) DeleteWatchRecord(_ context.Context, userID string, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.watchRecords[id]
	if !ok || record.UserID != userID {
		return ErrNotFound
	}
	delete(r.watchKeys, record.UserID+"\x00"+record.MediaKey)
	delete(r.watchRecords, id)
	return nil
}

func memoryFavoriteKey(userID, sourceID, mediaPath string) string {
	return userID + "\x00" + sourceID + "\x00" + mediaPath
}

func (r *MemoryRepository) AddDanmaku(_ context.Context, message DanmakuMessage) (DanmakuMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	message.ID = r.nextDanmaku
	r.nextDanmaku++
	message.CreatedAt = time.Now().UnixMilli()
	r.danmaku = append(r.danmaku, message)
	return message, nil
}

func (r *MemoryRepository) ListDanmaku(
	_ context.Context,
	fingerprint string,
	viewerID string,
	from, to float64,
	limit int,
) ([]DanmakuMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]DanmakuMessage, 0)
	for _, message := range r.danmaku {
		_, viewerBlocks := r.blocks[viewerID][message.UserID]
		_, blockedBy := r.blocks[message.UserID][viewerID]
		if message.Fingerprint == fingerprint && message.Position >= from && message.Position <= to &&
			!viewerBlocks && !blockedBy {
			result = append(result, message)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Position == result[j].Position {
			return result[i].ID < result[j].ID
		}
		return result[i].Position < result[j].Position
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) AddRoomMessage(_ context.Context, message ChatMessage) (ChatMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	message.ID = r.nextMessageID
	r.nextMessageID++
	if message.CreatedAt == 0 {
		message.CreatedAt = time.Now().UnixMilli()
	}
	r.messages = append(r.messages, message)
	return message, nil
}

func (r *MemoryRepository) ListRoomMessages(
	_ context.Context,
	roomCode string,
	viewerID string,
	before int64,
	limit int,
) ([]ChatMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ChatMessage, 0, limit)
	for index := len(r.messages) - 1; index >= 0 && len(result) < limit; index-- {
		message := r.messages[index]
		blocked := r.blocks[viewerID] != nil && r.blocks[viewerID][message.UserID].IsZero() == false
		blockedBy := r.blocks[message.UserID] != nil && r.blocks[message.UserID][viewerID].IsZero() == false
		if message.RoomCode == roomCode && !blocked && !blockedBy &&
			(before == 0 || message.ID < before) {
			result = append(result, message)
		}
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result, nil
}

func (r *MemoryRepository) GetPrivacy(_ context.Context, userID string) (PrivacySettings, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	settings, ok := r.privacy[userID]
	if !ok {
		return PrivacySettings{
			AllowRoomChat: true, AllowPrivateChat: true,
			AllowProfileFind: true, ShowWatchActivity: true,
		}, nil
	}
	return settings, nil
}

func (r *MemoryRepository) UpdatePrivacy(_ context.Context, userID string, settings PrivacySettings) error {
	r.mu.Lock()
	r.privacy[userID] = settings
	r.mu.Unlock()
	return nil
}

func (r *MemoryRepository) BlockUser(_ context.Context, blockerID, blockedID string) error {
	if blockerID == blockedID {
		return errors.New("cannot block self")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.usersByID[blockedID]; !ok {
		return ErrNotFound
	}
	if r.blocks[blockerID] == nil {
		r.blocks[blockerID] = make(map[string]time.Time)
	}
	r.blocks[blockerID][blockedID] = time.Now()
	return nil
}

func (r *MemoryRepository) UnblockUser(_ context.Context, blockerID, blockedID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.blocks[blockerID], blockedID)
	return nil
}

func (r *MemoryRepository) ListBlockedUsers(_ context.Context, userID string) ([]User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	users := make([]User, 0, len(r.blocks[userID]))
	for blockedID := range r.blocks[userID] {
		if account, ok := r.usersByID[blockedID]; ok {
			users = append(users, User{ID: account.ID, DisplayName: account.DisplayName})
		}
	}
	sort.Slice(users, func(i, j int) bool { return users[i].DisplayName < users[j].DisplayName })
	return users, nil
}

func (r *MemoryRepository) UsersBlocked(_ context.Context, userA, userB string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, aBlocks := r.blocks[userA][userB]
	_, bBlocks := r.blocks[userB][userA]
	return aBlocks || bBlocks, nil
}

func (r *MemoryRepository) CreateReport(_ context.Context, report Report) (Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	report.ID = r.nextReportID
	r.nextReportID++
	report.Status = "pending"
	report.CreatedAt = time.Now().UnixMilli()
	r.reports = append(r.reports, report)
	return report, nil
}

func (r *MemoryRepository) ListReports(_ context.Context, status string, before int64, limit int) ([]Report, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Report, 0, limit)
	for index := len(r.reports) - 1; index >= 0 && len(result) < limit; index-- {
		report := r.reports[index]
		if (status == "" || report.Status == status) && (before == 0 || report.ID < before) {
			result = append(result, report)
		}
	}
	return result, nil
}

func (r *MemoryRepository) ResolveReport(_ context.Context, id int64, reviewerID, status, resolution string) (Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := range r.reports {
		if r.reports[index].ID == id {
			r.reports[index].Status = status
			r.reports[index].Resolution = resolution
			r.reports[index].ReviewedBy = reviewerID
			r.reports[index].ReviewedAt = time.Now().UnixMilli()
			r.appendAuditLocked(AuditEvent{
				ActorID: reviewerID, Action: "report.resolve", TargetType: "report",
				TargetID: strconv.FormatInt(id, 10), Metadata: map[string]any{"status": status},
			})
			return r.reports[index], nil
		}
	}
	return Report{}, ErrNotFound
}

func (r *MemoryRepository) CloseRoom(_ context.Context, code, actorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[code]
	if !ok {
		return ErrNotFound
	}
	room.Closed = true
	r.rooms[code] = room
	r.appendAuditLocked(AuditEvent{
		ActorID: actorID, Action: "room.close", TargetType: "room", TargetID: code,
	})
	return nil
}

func (r *MemoryRepository) BanDevice(_ context.Context, deviceHash, reason, actorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	device, ok := r.devices[deviceHash]
	if !ok {
		return ErrNotFound
	}
	device.banned = true
	r.devices[deviceHash] = device
	for userID, links := range r.userDevices {
		if link, ok := links[deviceHash]; ok {
			link.revoked = true
			links[deviceHash] = link
			for hash, token := range r.refresh {
				if token.userID == userID && token.deviceHash == deviceHash {
					token.revoked = true
					r.refresh[hash] = token
				}
			}
		}
	}
	r.appendAuditLocked(AuditEvent{
		ActorID: actorID, Action: "device.ban", TargetType: "device", TargetID: deviceHash,
	})
	return nil
}

func (r *MemoryRepository) SetUserAdmin(_ context.Context, userID string, value bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.usersByID[userID]
	if !ok {
		return ErrNotFound
	}
	account.IsAdmin = value
	if value {
		account.AdminRole = "super_admin"
	} else {
		account.AdminRole = ""
	}
	r.usersByID[userID] = account
	return nil
}

func (r *MemoryRepository) SetUserVIP(_ context.Context, userID string, expiresAt int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.usersByID[userID]
	if !ok {
		return ErrNotFound
	}
	account.VIPExpiresAt = expiresAt
	r.usersByID[userID] = account
	return nil
}

func (r *MemoryRepository) AppendAudit(_ context.Context, event AuditEvent) (AuditEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendAuditLocked(event), nil
}

func (r *MemoryRepository) appendAuditLocked(event AuditEvent) AuditEvent {
	if len(r.audit) > 0 {
		event.PreviousHash = r.audit[len(r.audit)-1].EntryHash
	} else {
		event.PreviousHash = strings.Repeat("0", 64)
	}
	event.ID = r.nextAuditID
	r.nextAuditID++
	event.CreatedAt = time.Now().UnixMilli()
	event = finalizeAuditEvent(event)
	r.audit = append(r.audit, event)
	return event
}

func (r *MemoryRepository) ListAudit(_ context.Context, before int64, limit int) ([]AuditEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]AuditEvent, 0, limit)
	for index := len(r.audit) - 1; index >= 0 && len(result) < limit; index-- {
		event := r.audit[index]
		if before == 0 || event.ID < before {
			result = append(result, event)
		}
	}
	return result, nil
}

func (r *MemoryRepository) SearchSocialProfiles(
	_ context.Context, viewerID, query string, limit int,
) ([]SocialProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]SocialProfile, 0)
	for id, account := range r.usersByID {
		if id == viewerID || memoryUsersBlocked(r.blocks, viewerID, id) {
			continue
		}
		privacy, ok := r.privacy[id]
		if ok && !privacy.AllowProfileFind {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(account.DisplayName), query) &&
			strings.ToLower(account.Email) != query {
			continue
		}
		result = append(result, r.socialProfileLocked(viewerID, id))
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].DisplayName) < strings.ToLower(result[j].DisplayName)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) GetSocialProfile(_ context.Context, viewerID, targetID string) (SocialProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.usersByID[targetID]; !ok || memoryUsersBlocked(r.blocks, viewerID, targetID) {
		return SocialProfile{}, ErrNotFound
	}
	return r.socialProfileLocked(viewerID, targetID), nil
}

func (r *MemoryRepository) FollowUser(_ context.Context, followerID, followedID string) error {
	if followerID == followedID {
		return errors.New("cannot follow self")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.usersByID[followedID]; !ok || memoryUsersBlocked(r.blocks, followerID, followedID) {
		return ErrNotFound
	}
	if r.follows[followerID] == nil {
		r.follows[followerID] = make(map[string]time.Time)
	}
	r.follows[followerID][followedID] = time.Now()
	return nil
}

func (r *MemoryRepository) UnfollowUser(_ context.Context, followerID, followedID string) error {
	r.mu.Lock()
	delete(r.follows[followerID], followedID)
	r.mu.Unlock()
	return nil
}

func (r *MemoryRepository) CreateConversation(_ context.Context, userID, peerID string) (Conversation, error) {
	if userID == peerID {
		return Conversation{}, errors.New("cannot message self")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.usersByID[peerID]; !ok || memoryUsersBlocked(r.blocks, userID, peerID) {
		return Conversation{}, ErrNotFound
	}
	low, high := socialPair(userID, peerID)
	key := low + "\x00" + high
	id, ok := r.conversationKeys[key]
	if !ok {
		id = r.nextConversation
		r.nextConversation++
		now := time.Now()
		r.conversations[id] = memoryConversation{id: id, userLow: low, userHigh: high, updated: now}
		r.conversationKeys[key] = id
		r.conversationReads[id] = map[string]int64{low: 0, high: 0}
	}
	return r.memoryConversationViewLocked(userID, r.conversations[id]), nil
}

func (r *MemoryRepository) ListConversations(_ context.Context, userID string, limit int) ([]Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Conversation, 0)
	for _, conversation := range r.conversations {
		if conversation.userLow == userID || conversation.userHigh == userID {
			peer := conversation.userLow
			if peer == userID {
				peer = conversation.userHigh
			}
			if !memoryUsersBlocked(r.blocks, userID, peer) {
				result = append(result, r.memoryConversationViewLocked(userID, conversation))
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LastMessageAt > result[j].LastMessageAt })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) ListDirectMessages(
	_ context.Context, userID string, conversationID, before int64, limit int,
) ([]DirectMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conversation, ok := r.conversations[conversationID]
	if !ok || (conversation.userLow != userID && conversation.userHigh != userID) {
		return nil, ErrNotFound
	}
	peer := conversation.userLow
	if peer == userID {
		peer = conversation.userHigh
	}
	if memoryUsersBlocked(r.blocks, userID, peer) {
		return nil, ErrForbidden
	}
	messages := r.directMessages[conversationID]
	result := make([]DirectMessage, 0, limit)
	for index := len(messages) - 1; index >= 0 && len(result) < limit; index-- {
		if before == 0 || messages[index].ID < before {
			result = append(result, messages[index])
		}
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result, nil
}

func (r *MemoryRepository) AddDirectMessage(
	_ context.Context, senderID string, conversationID int64, body string,
) (DirectMessage, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	conversation, ok := r.conversations[conversationID]
	if !ok || (conversation.userLow != senderID && conversation.userHigh != senderID) {
		return DirectMessage{}, "", ErrNotFound
	}
	recipientID := conversation.userLow
	if recipientID == senderID {
		recipientID = conversation.userHigh
	}
	if memoryUsersBlocked(r.blocks, senderID, recipientID) {
		return DirectMessage{}, "", ErrForbidden
	}
	message := DirectMessage{
		ID: r.nextDirectMessage, ConversationID: conversationID,
		SenderID: senderID, Body: body, CreatedAt: time.Now().UnixMilli(),
	}
	r.nextDirectMessage++
	r.directMessages[conversationID] = append(r.directMessages[conversationID], message)
	conversation.updated = time.Now()
	r.conversations[conversationID] = conversation
	r.conversationReads[conversationID][senderID] = message.ID
	return message, recipientID, nil
}

func (r *MemoryRepository) ConversationPeer(_ context.Context, userID string, conversationID int64) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conversation, ok := r.conversations[conversationID]
	if !ok || (conversation.userLow != userID && conversation.userHigh != userID) {
		return "", ErrNotFound
	}
	if conversation.userLow == userID {
		return conversation.userHigh, nil
	}
	return conversation.userLow, nil
}

func (r *MemoryRepository) MarkConversationRead(_ context.Context, userID string, conversationID, messageID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	conversation, ok := r.conversations[conversationID]
	if !ok || (conversation.userLow != userID && conversation.userHigh != userID) {
		return ErrNotFound
	}
	if messageID > r.conversationReads[conversationID][userID] {
		r.conversationReads[conversationID][userID] = messageID
	}
	return nil
}

func (r *MemoryRepository) UnreadDirectCount(_ context.Context, userID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for id, conversation := range r.conversations {
		if conversation.userLow != userID && conversation.userHigh != userID {
			continue
		}
		peer := conversation.userLow
		if peer == userID {
			peer = conversation.userHigh
		}
		if memoryUsersBlocked(r.blocks, userID, peer) {
			continue
		}
		lastRead := r.conversationReads[id][userID]
		for _, message := range r.directMessages[id] {
			if message.SenderID != userID && message.ID > lastRead {
				count++
			}
		}
	}
	return count, nil
}

func (r *MemoryRepository) socialProfileLocked(viewerID, targetID string) SocialProfile {
	account := r.usersByID[targetID]
	profile := SocialProfile{
		ID: targetID, DisplayName: account.DisplayName, Signature: account.Signature,
	}
	_, profile.Following = r.follows[viewerID][targetID]
	_, profile.FollowsViewer = r.follows[targetID][viewerID]
	profile.FollowingCount = len(r.follows[targetID])
	for _, followed := range r.follows {
		if _, ok := followed[targetID]; ok {
			profile.FollowerCount++
		}
	}
	return profile
}

func (r *MemoryRepository) memoryConversationViewLocked(userID string, conversation memoryConversation) Conversation {
	peerID := conversation.userLow
	if peerID == userID {
		peerID = conversation.userHigh
	}
	view := Conversation{ID: conversation.id, Peer: r.socialProfileLocked(userID, peerID)}
	messages := r.directMessages[conversation.id]
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		view.LastMessage = last.Body
		view.LastMessageAt = last.CreatedAt
		lastRead := r.conversationReads[conversation.id][userID]
		for _, message := range messages {
			if message.SenderID != userID && message.ID > lastRead {
				view.UnreadCount++
			}
		}
	}
	return view
}

func memoryUsersBlocked(blocks map[string]map[string]time.Time, userA, userB string) bool {
	_, aBlocks := blocks[userA][userB]
	_, bBlocks := blocks[userB][userA]
	return aBlocks || bBlocks
}

func socialPair(userA, userB string) (string, string) {
	if userA < userB {
		return userA, userB
	}
	return userB, userA
}

func (r *MemoryRepository) SaveRoom(_ context.Context, room Room) error {
	r.mu.Lock()
	r.rooms[room.Code] = room
	r.mu.Unlock()
	return nil
}

func (r *MemoryRepository) SaveMember(_ context.Context, code string, member Member) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[code]
	if !ok {
		return ErrNotFound
	}
	for _, existing := range room.Members {
		if existing.UserID == member.UserID {
			return nil
		}
	}
	room.Members = append(room.Members, member)
	r.rooms[code] = room
	return nil
}

func (r *MemoryRepository) UpdatePlayback(_ context.Context, code string, playback Playback) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[code]
	if !ok {
		return ErrNotFound
	}
	room.Playback = playback
	r.rooms[code] = room
	return nil
}

func (r *MemoryRepository) LoadRooms(_ context.Context) ([]Room, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rooms := make([]Room, 0, len(r.rooms))
	for _, room := range r.rooms {
		room.Members = append([]Member(nil), room.Members...)
		rooms = append(rooms, room)
	}
	return rooms, nil
}

func (r *MemoryRepository) Close() {}
