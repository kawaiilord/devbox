package app

import (
	"context"
	"errors"
	"sort"
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
	AddRoomMessage(context.Context, ChatMessage) (ChatMessage, error)
	ListRoomMessages(context.Context, string, int64, int) ([]ChatMessage, error)
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
	mu            sync.RWMutex
	usersByID     map[string]AccountRecord
	usersByMail   map[string]string
	refresh       map[string]memoryRefreshToken
	devices       map[string]memoryDevice
	userDevices   map[string]map[string]memoryUserDevice
	actionTokens  map[string]memoryActionToken
	mediaSources  map[string]MediaSource
	messages      []ChatMessage
	nextMessageID int64
	rooms         map[string]Room
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		usersByID:     make(map[string]AccountRecord),
		usersByMail:   make(map[string]string),
		refresh:       make(map[string]memoryRefreshToken),
		devices:       make(map[string]memoryDevice),
		userDevices:   make(map[string]map[string]memoryUserDevice),
		actionTokens:  make(map[string]memoryActionToken),
		mediaSources:  make(map[string]MediaSource),
		nextMessageID: 1,
		rooms:         make(map[string]Room),
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
	return nil
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
	before int64,
	limit int,
) ([]ChatMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ChatMessage, 0, limit)
	for index := len(r.messages) - 1; index >= 0 && len(result) < limit; index-- {
		message := r.messages[index]
		if message.RoomCode == roomCode && (before == 0 || message.ID < before) {
			result = append(result, message)
		}
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result, nil
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
