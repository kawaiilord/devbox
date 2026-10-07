package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	ErrEmailExists        = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh     = errors.New("invalid or expired refresh token")
)

type AccountRecord struct {
	User
	PasswordHash string
	CreatedAt    time.Time
}

type Repository interface {
	CreateUser(context.Context, AccountRecord) error
	UserByEmail(context.Context, string) (AccountRecord, error)
	UserByID(context.Context, string) (AccountRecord, error)
	StoreRefreshToken(context.Context, string, string, time.Time) error
	RotateRefreshToken(context.Context, string, string, time.Time) (AccountRecord, error)
	RevokeRefreshToken(context.Context, string) error
	SaveRoom(context.Context, Room) error
	SaveMember(context.Context, string, Member) error
	UpdatePlayback(context.Context, string, Playback) error
	LoadRooms(context.Context) ([]Room, error)
	Close()
}

type memoryRefreshToken struct {
	userID    string
	expiresAt time.Time
	revoked   bool
}

type MemoryRepository struct {
	mu          sync.RWMutex
	usersByID   map[string]AccountRecord
	usersByMail map[string]string
	refresh     map[string]memoryRefreshToken
	rooms       map[string]Room
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		usersByID:   make(map[string]AccountRecord),
		usersByMail: make(map[string]string),
		refresh:     make(map[string]memoryRefreshToken),
		rooms:       make(map[string]Room),
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

func (r *MemoryRepository) StoreRefreshToken(_ context.Context, userID, hash string, expiresAt time.Time) error {
	r.mu.Lock()
	r.refresh[hash] = memoryRefreshToken{userID: userID, expiresAt: expiresAt}
	r.mu.Unlock()
	return nil
}

func (r *MemoryRepository) RotateRefreshToken(_ context.Context, oldHash, newHash string, expiresAt time.Time) (AccountRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.refresh[oldHash]
	if !ok || token.revoked || !token.expiresAt.After(time.Now()) {
		return AccountRecord{}, ErrInvalidRefresh
	}
	token.revoked = true
	r.refresh[oldHash] = token
	r.refresh[newHash] = memoryRefreshToken{userID: token.userID, expiresAt: expiresAt}
	account, ok := r.usersByID[token.userID]
	if !ok {
		return AccountRecord{}, ErrInvalidRefresh
	}
	return account, nil
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
