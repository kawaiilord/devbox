package app

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrRoomFull     = errors.New("room is full")
	ErrExpired      = errors.New("room has expired")
	ErrStaleControl = errors.New("stale control sequence")
)

type roomRecord struct {
	room           Room
	members        map[string]Member
	lastControlSeq map[string]int64
	serverSeq      int64
}

type socketTicket struct {
	user      User
	roomCode  string
	expiresAt time.Time
}

type Store struct {
	mu       sync.RWMutex
	rooms    map[string]*roomRecord
	sessions map[string]User
	tickets  map[string]socketTicket
	now      func() time.Time
}

func NewStore() *Store {
	return &Store{
		rooms:    make(map[string]*roomRecord),
		sessions: make(map[string]User),
		tickets:  make(map[string]socketTicket),
		now:      time.Now,
	}
}

func (s *Store) IssueSocketTicket(code string, user User) (string, error) {
	code = strings.ToUpper(code)
	ticket, err := randomString(24)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.rooms[code]
	if !ok {
		return "", ErrNotFound
	}
	if _, member := record.members[user.ID]; !member {
		return "", ErrForbidden
	}
	if record.room.ExpiresAt <= s.now().UnixMilli() {
		return "", ErrExpired
	}
	s.tickets[ticket] = socketTicket{
		user: user, roomCode: code, expiresAt: s.now().Add(30 * time.Second),
	}
	return ticket, nil
}

func (s *Store) ConsumeSocketTicket(ticket, code string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	issued, ok := s.tickets[ticket]
	delete(s.tickets, ticket)
	if !ok || issued.roomCode != strings.ToUpper(code) || !issued.expiresAt.After(s.now()) {
		return User{}, ErrUnauthorized
	}
	return issued.user, nil
}

func (s *Store) CreateSession(displayName string) (Session, error) {
	displayName = strings.TrimSpace(displayName)
	if len([]rune(displayName)) < 2 || len([]rune(displayName)) > 32 {
		return Session{}, fmt.Errorf("display name must be 2-32 characters")
	}
	userID, err := randomString(12)
	if err != nil {
		return Session{}, err
	}
	token, err := randomString(32)
	if err != nil {
		return Session{}, err
	}
	user := User{ID: userID, DisplayName: displayName}
	s.mu.Lock()
	s.sessions[token] = user
	s.mu.Unlock()
	return Session{AccessToken: token, User: user}, nil
}

func (s *Store) Authenticate(token string) (User, error) {
	s.mu.RLock()
	user, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok {
		return User{}, ErrUnauthorized
	}
	return user, nil
}

func (s *Store) CreateRoom(owner User, name, sourceURL string, maxMembers int) (Room, error) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) < 2 || len([]rune(name)) > 64 {
		return Room{}, fmt.Errorf("room name must be 2-64 characters")
	}
	parsed, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Room{}, fmt.Errorf("source_url must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil {
		return Room{}, fmt.Errorf("source_url must not contain credentials")
	}
	if maxMembers == 0 {
		maxMembers = 8
	}
	if maxMembers < 2 || maxMembers > 100 {
		return Room{}, fmt.Errorf("max_members must be between 2 and 100")
	}
	now := s.now()
	member := Member{UserID: owner.ID, DisplayName: owner.DisplayName, JoinedAt: now.UnixMilli()}

	s.mu.Lock()
	defer s.mu.Unlock()
	var code string
	for attempts := 0; attempts < 10; attempts++ {
		code, err = roomCode()
		if err != nil {
			return Room{}, err
		}
		if _, exists := s.rooms[code]; !exists {
			break
		}
	}
	if _, exists := s.rooms[code]; exists {
		return Room{}, errors.New("failed to allocate room code")
	}
	record := &roomRecord{
		room: Room{
			Code:       code,
			Name:       name,
			OwnerID:    owner.ID,
			SourceURL:  parsed.String(),
			MaxMembers: maxMembers,
			CreatedAt:  now.UnixMilli(),
			ExpiresAt:  now.Add(6 * time.Hour).UnixMilli(),
			Playback: Playback{
				Speed:         1,
				PositionTS:    now.UnixMilli(),
				SourceVersion: 1,
			},
		},
		members:        map[string]Member{owner.ID: member},
		lastControlSeq: make(map[string]int64),
	}
	s.rooms[code] = record
	return cloneRoom(record, now), nil
}

func (s *Store) JoinRoom(code string, user User) (Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.rooms[strings.ToUpper(code)]
	if !ok {
		return Room{}, ErrNotFound
	}
	now := s.now()
	if record.room.ExpiresAt <= now.UnixMilli() {
		return Room{}, ErrExpired
	}
	if _, exists := record.members[user.ID]; !exists {
		if len(record.members) >= record.room.MaxMembers {
			return Room{}, ErrRoomFull
		}
		record.members[user.ID] = Member{
			UserID: user.ID, DisplayName: user.DisplayName, JoinedAt: now.UnixMilli(),
		}
	}
	return cloneRoom(record, now), nil
}

func (s *Store) GetRoom(code, userID string) (Room, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.rooms[strings.ToUpper(code)]
	if !ok {
		return Room{}, ErrNotFound
	}
	if _, member := record.members[userID]; !member {
		return Room{}, ErrForbidden
	}
	now := s.now()
	if record.room.ExpiresAt <= now.UnixMilli() {
		return Room{}, ErrExpired
	}
	return cloneRoom(record, now), nil
}

func (s *Store) ApplyControl(code string, user User, clientSeq int64, control Control) (Room, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.rooms[strings.ToUpper(code)]
	if !ok {
		return Room{}, 0, ErrNotFound
	}
	if record.room.OwnerID != user.ID {
		return Room{}, 0, ErrForbidden
	}
	if clientSeq <= record.lastControlSeq[user.ID] {
		return Room{}, 0, ErrStaleControl
	}
	now := s.now()
	playback := projected(record.room.Playback, now)
	switch control.Action {
	case "play":
		playback.Playing = true
	case "pause":
		playback.Playing = false
	case "seek":
		if control.Position == nil || *control.Position < 0 {
			return Room{}, 0, fmt.Errorf("seek requires a non-negative position")
		}
		playback.Position = *control.Position
	case "speed":
		if control.Speed == nil || *control.Speed < 0.5 || *control.Speed > 2 {
			return Room{}, 0, fmt.Errorf("speed must be between 0.5 and 2")
		}
		playback.Speed = *control.Speed
	case "episode":
		if control.Episode == nil || *control.Episode < 0 {
			return Room{}, 0, fmt.Errorf("episode requires a non-negative index")
		}
		playback.Episode = *control.Episode
		playback.Position = 0
	default:
		return Room{}, 0, fmt.Errorf("unsupported action %q", control.Action)
	}
	playback.PositionTS = now.UnixMilli()
	record.room.Playback = playback
	record.lastControlSeq[user.ID] = clientSeq
	record.serverSeq++
	return cloneRoom(record, now), record.serverSeq, nil
}

func (s *Store) Snapshot(code string) (Room, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.rooms[strings.ToUpper(code)]
	if !ok {
		return Room{}, 0, ErrNotFound
	}
	record.serverSeq++
	return cloneRoom(record, s.now()), record.serverSeq, nil
}

func (s *Store) RoomCodes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	codes := make([]string, 0, len(s.rooms))
	for code := range s.rooms {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

func cloneRoom(record *roomRecord, now time.Time) Room {
	room := record.room
	room.Playback = projected(room.Playback, now)
	room.Members = make([]Member, 0, len(record.members))
	for _, member := range record.members {
		room.Members = append(room.Members, member)
	}
	sort.Slice(room.Members, func(i, j int) bool { return room.Members[i].JoinedAt < room.Members[j].JoinedAt })
	return room
}

func randomString(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func roomCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buffer := make([]byte, 6)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	for i := range buffer {
		buffer[i] = alphabet[int(buffer[i])%len(alphabet)]
	}
	return string(buffer), nil
}
