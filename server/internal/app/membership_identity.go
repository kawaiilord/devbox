package app

import (
	"context"
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
