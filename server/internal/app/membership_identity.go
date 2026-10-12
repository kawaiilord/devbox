package app

import (
	"context"
	"encoding/json"
	"time"
)

func (r *MemoryRepository) ActiveMemberships(_ context.Context, ids []string) (map[string]bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]bool, len(ids))
	now := time.Now().UnixMilli()
	for _, id := range ids {
		result[id] = r.usersByID[id].VIPExpiresAt > now
	}
	return result, nil
}

func (r *PostgresRepository) ActiveMemberships(ctx context.Context, ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT id,COALESCE(vip_expires_at>now(),false) FROM users WHERE id=ANY($1::text[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var active bool
		if err := rows.Scan(&id, &active); err != nil {
			return nil, err
		}
		result[id] = active
	}
	return result, rows.Err()
}

// Membership metadata is refreshed separately from playback so expiry changes
// never reload a movie or alter its synchronization sequence.
func (s *Server) broadcastMembershipStatus(ctx context.Context, room Room) {
	ids := make([]string, 0, len(room.Members))
	for _, member := range room.Members {
		ids = append(ids, member.UserID)
	}
	values, err := s.repo.ActiveMemberships(ctx, ids)
	if err != nil {
		return
	}
	payload, _ := json.Marshal(map[string]any{"memberships": values})
	s.emitEnvelope(ctx, Envelope{Type: "room.membership", Room: room.Code, TS: time.Now().UnixMilli(), Payload: payload, From: "server"})
}

func (s *Server) publishUserMembership(ctx context.Context, userID string) {
	for _, code := range s.store.RoomCodes() {
		room, err := s.store.Room(code)
		if err != nil {
			continue
		}
		for _, member := range room.Members {
			if member.UserID == userID {
				s.broadcastMembershipStatus(ctx, room)
				break
			}
		}
	}
}
