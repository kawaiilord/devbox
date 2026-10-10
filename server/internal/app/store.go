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
	ErrRoomClosed   = errors.New("room is closed")
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
	mu      sync.RWMutex
	rooms   map[string]*roomRecord
	tickets map[string]socketTicket
	now     func() time.Time
}

func NewStore() *Store {
	return &Store{
		rooms:   make(map[string]*roomRecord),
		tickets: make(map[string]socketTicket),
		now:     time.Now,
	}
}

func (s *Store) RestoreRooms(rooms []Room) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, room := range rooms {
		members := make(map[string]Member, len(room.Members))
		for _, member := range room.Members {
			members[member.UserID] = member
		}
		room.Members = nil
		s.rooms[room.Code] = &roomRecord{
			room: room, members: members, lastControlSeq: make(map[string]int64),
		}
	}
}

func (s *Store) UpsertAuthoritativeRoom(room Room, serverSeq int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.rooms[room.Code]
	if existing != nil {
		if existing.room.Closed && !room.Closed {
			return
		}
		if serverSeq < existing.serverSeq && !room.Closed {
			return
		}
	}
	members := make(map[string]Member, len(room.Members))
	for _, member := range room.Members {
		members[member.UserID] = member
	}
	room.Members = nil
	lastControlSeq := make(map[string]int64)
	if existing != nil {
		lastControlSeq = existing.lastControlSeq
	}
	s.rooms[room.Code] = &roomRecord{
		room: room, members: members, lastControlSeq: lastControlSeq, serverSeq: serverSeq,
	}
}

func (s *Store) ApplyAuthoritativePlayback(code string, playback Playback, serverSeq int64) (Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.rooms[strings.ToUpper(code)]
	if !ok {
		return Room{}, ErrNotFound
	}
	if record.room.Closed {
		return cloneRoom(record, s.now()), nil
	}
	if serverSeq < record.serverSeq {
		return cloneRoom(record, s.now()), nil
	}
	record.room.Playback = playback
	record.serverSeq = serverSeq
	return cloneRoom(record, s.now()), nil
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
	if record.room.Closed {
		return "", ErrRoomClosed
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

func (s *Store) IssueIdentityTicket(user User) (string, error) {
	ticket, err := randomString(24)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.tickets[ticket] = socketTicket{user: user, roomCode: "@SOCIAL", expiresAt: s.now().Add(30 * time.Second)}
	s.mu.Unlock()
	return ticket, nil
}

func (s *Store) ConsumeIdentityTicket(ticket string) (User, error) {
	return s.ConsumeSocketTicket(ticket, "@SOCIAL")
}

func (s *Store) CreateRoom(owner User, name, sourceURL string, maxMembers int) (Room, error) {
	return s.createRoom(owner, name, sourceURL, "", "", maxMembers)
}

func (s *Store) CreateMediaRoom(
	owner User,
	name, sourceID, mediaPath string,
	maxMembers int,
) (Room, error) {
	if sourceID == "" || cleanMediaPath(mediaPath) == "/" {
		return Room{}, errors.New("media source and file path are required")
	}
	return s.createRoom(owner, name, "", sourceID, cleanMediaPath(mediaPath), maxMembers)
}

func (s *Store) createRoom(
	owner User,
	name, sourceURL, mediaSourceID, mediaPath string,
	maxMembers int,
) (Room, error) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) < 2 || len([]rune(name)) > 64 {
		return Room{}, fmt.Errorf("room name must be 2-64 characters")
	}
	parsed := &url.URL{}
	var err error
	if mediaSourceID == "" {
		parsed, err = url.Parse(strings.TrimSpace(sourceURL))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return Room{}, fmt.Errorf("source_url must be an absolute HTTP(S) URL")
		}
		if parsed.User != nil {
			return Room{}, fmt.Errorf("source_url must not contain credentials")
		}
	}
	if maxMembers == 0 {
		maxMembers = 8
	}
	if maxMembers < 2 || maxMembers > 100 {
		return Room{}, fmt.Errorf("max_members must be between 2 and 100")
	}
	now := s.now()
	expiresAt := now.Add(6 * time.Hour)
	if owner.VIPExpiresAt > now.UnixMilli() {
		expiresAt = time.UnixMilli(owner.VIPExpiresAt)
	}
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
			Code:          code,
			Name:          name,
			OwnerID:       owner.ID,
			SourceURL:     parsed.String(),
			MediaSourceID: mediaSourceID,
			MediaPath:     mediaPath,
			MaxMembers:    maxMembers,
			CreatedAt:     now.UnixMilli(),
			ExpiresAt:     expiresAt.UnixMilli(),
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
	if record.room.Closed {
		return Room{}, ErrRoomClosed
	}
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
	if record.room.Closed {
		return Room{}, ErrRoomClosed
	}
	now := s.now()
	if record.room.ExpiresAt <= now.UnixMilli() {
		return Room{}, ErrExpired
	}
	return cloneRoom(record, now), nil
}

func (s *Store) Room(code string) (Room, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.rooms[strings.ToUpper(code)]
	if !ok {
		return Room{}, ErrNotFound
	}
	return cloneRoom(record, s.now()), nil
}

func (s *Store) CloseRoom(code string) (Room, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.rooms[strings.ToUpper(code)]
	if !ok {
		return Room{}, 0, ErrNotFound
	}
	record.room.Closed = true
	record.room.Playback.Playing = false
	record.room.Playback.PositionTS = s.now().UnixMilli()
	record.serverSeq++
	return cloneRoom(record, s.now()), record.serverSeq, nil
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
	if record.room.Closed {
		return Room{}, 0, ErrRoomClosed
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
		if !s.rooms[code].room.Closed {
			codes = append(codes, code)
		}
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
