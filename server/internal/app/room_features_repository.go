package app

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *MemoryRepository) GetRoomFeatures(_ context.Context, code string) (RoomFeatures, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if f, ok := r.roomFeatures[code]; ok {
		return cloneRoomFeatures(f), nil
	}
	return defaultRoomFeatures(), nil
}

func (r *MemoryRepository) SaveRoomFeatures(_ context.Context, code string, expected int64, f RoomFeatures, media *RoomMediaSelection) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[code]
	if !ok {
		return ErrNotFound
	}
	if r.roomFeatures[code].Version != expected || f.Version != expected+1 {
		return ErrRoomConflict
	}
	if r.roomFeatures == nil {
		r.roomFeatures = map[string]RoomFeatures{}
	}
	r.roomFeatures[code] = cloneRoomFeatures(f)
	if media != nil {
		room.MediaSourceID = media.MediaSourceID
		room.MediaPath = media.MediaPath
		room.SourceURL = media.SourceURL
		room.Playback = media.Playback
		r.rooms[code] = room
	}
	return nil
}

func (r *MemoryRepository) DiscoverRooms(_ context.Context, q RoomDiscoveryQuery) ([]PublicRoom, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rooms := []Room{}
	for _, room := range r.rooms {
		if room.ExpiresAt > time.Now().UnixMilli() && roomDiscoveryMatches(room, r.roomFeatures[room.Code], q) {
			rooms = append(rooms, room)
		}
	}
	sort.Slice(rooms, func(i, j int) bool {
		if rooms[i].CreatedAt == rooms[j].CreatedAt {
			return rooms[i].Code < rooms[j].Code
		}
		return rooms[i].CreatedAt > rooms[j].CreatedAt
	})
	result := []PublicRoom{}
	for i := q.Offset; i < len(rooms) && len(result) < q.Limit; i++ {
		result = append(result, publicRoom(rooms[i], r.roomFeatures[rooms[i].Code]))
	}
	return result, nil
}

func (r *MemoryRepository) DeleteRoomMember(_ context.Context, code, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[code]
	if !ok {
		return ErrNotFound
	}
	if room.OwnerID == userID {
		return ErrForbidden
	}
	members := make([]Member, 0, len(room.Members))
	for _, m := range room.Members {
		if m.UserID != userID {
			members = append(members, m)
		}
	}
	room.Members = members
	r.rooms[code] = room
	return nil
}

func (r *PostgresRepository) GetRoomFeatures(ctx context.Context, code string) (RoomFeatures, error) {
	var data []byte
	var version int64
	err := r.pool.QueryRow(ctx, `SELECT version,data FROM room_features WHERE room_code=$1`, code).Scan(&version, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultRoomFeatures(), nil
	}
	if err != nil {
		return RoomFeatures{}, err
	}
	f := defaultRoomFeatures()
	if err = json.Unmarshal(data, &f); err != nil {
		return RoomFeatures{}, err
	}
	f.Version = version
	return f, nil
}

func (r *PostgresRepository) SaveRoomFeatures(ctx context.Context, code string, expected int64, f RoomFeatures, media *RoomMediaSelection) error {
	if f.Version != expected+1 {
		return ErrRoomConflict
	}
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	query := `UPDATE room_features SET version=$2,data=$3 WHERE room_code=$1 AND version=$4`
	if expected == 0 {
		query = `INSERT INTO room_features(room_code,version,data) SELECT $1,$2,$3 WHERE $4::bigint=0 ON CONFLICT DO NOTHING`
	}
	result, err := tx.Exec(ctx, query, code, f.Version, data, expected)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrRoomConflict
	}
	if media != nil {
		result, err = tx.Exec(ctx, `UPDATE rooms SET source_url=$2,media_source_id=$3,media_path=$4,
		position=$5,playing=$6,speed=$7,episode=$8,position_ts=$9,source_version=$10 WHERE code=$1 AND closed_at IS NULL`,
			code, media.SourceURL, nilIfEmpty(media.MediaSourceID), nilIfEmpty(media.MediaPath), media.Playback.Position, media.Playback.Playing,
			media.Playback.Speed, media.Playback.Episode, media.Playback.PositionTS, media.Playback.SourceVersion)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return ErrRoomClosed
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) DiscoverRooms(ctx context.Context, q RoomDiscoveryQuery) ([]PublicRoom, error) {
	rows, err := r.pool.Query(ctx, `SELECT trim(r.code),r.name,f.data,r.max_members,r.expires_at,
	 (SELECT count(*) FROM room_members m WHERE m.room_code=r.code)
	 FROM rooms r JOIN room_features f ON f.room_code=r.code
	 WHERE r.closed_at IS NULL AND r.expires_at>$1 AND f.data->>'visibility'='public'
	 AND ($2='' OR strpos(lower(r.name||' '||COALESCE(f.data->>'description','')),lower($2))>0)
	 AND ($3='' OR f.data->>'category'=$3) AND ($4='' OR f.data->'tags' ? $4)
	 ORDER BY r.created_at DESC,r.code ASC OFFSET $5 LIMIT $6`, time.Now().UnixMilli(), q.Search, q.Category, q.Tag, q.Offset, q.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PublicRoom{}
	for rows.Next() {
		var room PublicRoom
		var data []byte
		if err := rows.Scan(&room.Code, &room.Name, &data, &room.MaxMembers, &room.ExpiresAt, &room.Members); err != nil {
			return nil, err
		}
		f := defaultRoomFeatures()
		if err = json.Unmarshal(data, &f); err != nil {
			return nil, err
		}
		room.Description = f.Description
		room.Category = f.Category
		room.Tags = f.Tags
		room.AllowGuests = f.AllowGuests
		room.PasswordProtected = f.PasswordHash != ""
		result = append(result, room)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) DeleteRoomMember(ctx context.Context, code, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM room_members m USING rooms r WHERE m.room_code=r.code AND m.room_code=$1 AND m.user_id=$2 AND r.owner_id<>$2`, code, userID)
	return err
}

func normalizeDiscoveryQuery(search, category, tag string, offset, limit int) RoomDiscoveryQuery {
	if offset < 0 {
		offset = 0
	}
	if offset > 10000 {
		offset = 10000
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	return RoomDiscoveryQuery{Search: sanitizeAuditText(strings.TrimSpace(search), 100), Category: sanitizeAuditText(strings.TrimSpace(category), 40), Tag: sanitizeAuditText(strings.TrimSpace(tag), 30), Offset: offset, Limit: limit}
}
