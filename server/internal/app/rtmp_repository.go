package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
)

type RoomStream struct {
	RoomCode  string `json:"room_code"`
	OwnerID   string `json:"owner_id"`
	Path      string `json:"path"`
	SourceID  string `json:"source_id"`
	KeyHash   string `json:"key_hash"`
	ExpiresAt int64  `json:"expires_at"`
	Version   int64  `json:"version"`
	Revoked   bool   `json:"revoked"`
}
type StreamRepository interface {
	ReadRoomStream(context.Context, string) (RoomStream, error)
	WriteRoomStream(context.Context, RoomStream, string) (RoomStream, error)
}

func (r *MemoryRepository) ReadRoomStream(_ context.Context, code string) (RoomStream, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.roomStreams[code]
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}
func (r *MemoryRepository) WriteRoomStream(_ context.Context, v RoomStream, actor string) (RoomStream, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.usersByID[actor]
	room, exists := r.rooms[v.RoomCode]
	if !ok || !exists || !accountActive(a.User) || (room.OwnerID != actor && !permissionGranted(a.User, "room.manage")) {
		return v, ErrForbidden
	}
	if r.roomStreams[v.RoomCode].Version != v.Version {
		return v, ErrAdminConflict
	}
	v.Version++
	if r.roomStreams == nil {
		r.roomStreams = map[string]RoomStream{}
	}
	r.roomStreams[v.RoomCode] = v
	r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "rtmp.key.update", TargetType: "room", TargetID: v.RoomCode})
	return v, nil
}
func (r *PostgresRepository) ReadRoomStream(ctx context.Context, code string) (RoomStream, error) {
	var v RoomStream
	var raw []byte
	e := r.pool.QueryRow(ctx, `SELECT data FROM room_streams WHERE room_code=$1`, code).Scan(&raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &v)
	}
	return v, e
}
func (r *PostgresRepository) WriteRoomStream(ctx context.Context, v RoomStream, actor string) (RoomStream, error) {
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return v, e
	}
	defer tx.Rollback(ctx)
	var owner string
	if e = tx.QueryRow(ctx, `SELECT owner_id FROM rooms WHERE code=$1 FOR UPDATE`, v.RoomCode).Scan(&owner); e != nil {
		return v, e
	}
	a, e := scanAccount(tx.QueryRow(ctx, `SELECT `+adminAccountColumns+` FROM users WHERE id=$1 FOR SHARE`, actor))
	if e != nil || !accountActive(a.User) || (owner != actor && !permissionGranted(a.User, "room.manage")) {
		return v, ErrForbidden
	}
	var version int64
	e = tx.QueryRow(ctx, `SELECT version FROM room_streams WHERE room_code=$1`, v.RoomCode).Scan(&version)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return v, e
	}
	if version != v.Version {
		return v, ErrAdminConflict
	}
	v.Version++
	raw, _ := json.Marshal(v)
	if _, e = tx.Exec(ctx, `INSERT INTO room_streams(room_code,version,data) VALUES($1,$2,$3) ON CONFLICT(room_code) DO UPDATE SET version=EXCLUDED.version,data=EXCLUDED.data`, v.RoomCode, v.Version, raw); e != nil {
		return v, e
	}
	if _, e = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "rtmp.key.update", TargetType: "room", TargetID: v.RoomCode}); e != nil {
		return v, e
	}
	return v, tx.Commit(ctx)
}
