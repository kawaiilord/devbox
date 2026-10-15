package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
	"time"
)

const adminAccountColumns = `id,email,display_name,password_hash,created_at,email_verified_at IS NOT NULL,session_version,is_admin,signature,admin_role,COALESCE((extract(epoch FROM vip_expires_at)*1000)::bigint,0),account_status,status_reason,admin_version`

func postgresAdminActor(ctx context.Context, tx pgx.Tx, id, permission string) (AccountRecord, error) {
	a, err := scanAccount(tx.QueryRow(ctx, `SELECT `+adminAccountColumns+` FROM users WHERE id=$1 FOR SHARE`, id))
	if err != nil {
		return a, err
	}
	if !permissionGranted(a.User, permission) {
		return a, ErrForbidden
	}
	return a, nil
}
func (r *PostgresRepository) AdminFindUsers(ctx context.Context, q AdminQuery) ([]ManagedUser, int, error) {
	where := ` FROM users WHERE ($1='' OR strpos(lower(id||' '||email||' '||display_name),lower($1))>0) AND ($2='' OR account_status=$2)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*)`+where, q.Search, q.Status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+adminAccountColumns+where+` ORDER BY created_at DESC,id LIMIT $3 OFFSET $4`, q.Search, q.Status, q.Limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []ManagedUser{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, managedUser(a))
	}
	return items, total, rows.Err()
}
func (r *PostgresRepository) AdminCreateUser(ctx context.Context, a AccountRecord, actor string) (ManagedUser, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ManagedUser{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = postgresAdminActor(ctx, tx, actor, "user.manage"); err != nil {
		return ManagedUser{}, err
	}
	a.AccountStatus = "active"
	a.AdminVersion = 1
	_, err = tx.Exec(ctx, `INSERT INTO users(id,email,display_name,password_hash,created_at,email_verified_at) VALUES($1,$2,$3,$4,$5,CASE WHEN $6::boolean THEN now() ELSE NULL END)`, a.ID, a.Email, a.DisplayName, a.PasswordHash, a.CreatedAt, a.EmailVerified)
	if err != nil {
		var e *pgconn.PgError
		if errors.As(err, &e) && e.Code == "23505" {
			return ManagedUser{}, ErrEmailExists
		}
		return ManagedUser{}, err
	}
	if err = insertRegistrationTx(ctx, tx, a, defaultSitePolicy()); err != nil {
		return ManagedUser{}, err
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "user.create", TargetType: "user", TargetID: a.ID}); err != nil {
		return ManagedUser{}, err
	}
	return managedUser(a), tx.Commit(ctx)
}
func (r *PostgresRepository) AdminMutateUser(ctx context.Context, id, actor string, change AdminUserChange) (ManagedUser, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ManagedUser{}, err
	}
	defer tx.Rollback(ctx)
	// Lock both identities in a stable order before checking roles, including concurrent demotions.
	rows, err := tx.Query(ctx, `SELECT `+adminAccountColumns+` FROM users WHERE id=ANY($1) ORDER BY id FOR UPDATE`, []string{id, actor})
	if err != nil {
		return ManagedUser{}, err
	}
	records := map[string]AccountRecord{}
	for rows.Next() {
		a, e := scanAccount(rows)
		if e != nil {
			rows.Close()
			return ManagedUser{}, e
		}
		records[a.ID] = a
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return ManagedUser{}, err
	}
	a, ok := records[id]
	if !ok {
		return ManagedUser{}, ErrNotFound
	}
	if err = applyAdminUserChange(records[actor].User, &a, change); err != nil {
		return ManagedUser{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE users SET display_name=$2,password_hash=$3,admin_role=$4,is_admin=$5,account_status=$6,status_reason=$7,admin_version=$8,session_version=$9 WHERE id=$1`, id, a.DisplayName, a.PasswordHash, a.AdminRole, a.IsAdmin, a.AccountStatus, a.StatusReason, a.AdminVersion, a.SessionVersion)
	if err != nil {
		return ManagedUser{}, err
	}
	if accountChangeRevokes(change.Action) {
		if _, err = tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, id); err != nil {
			return ManagedUser{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE auth_action_tokens SET used_at=COALESCE(used_at,now()) WHERE user_id=$1`, id); err != nil {
			return ManagedUser{}, err
		}
	}
	if change.Action == "profile" {
		if _, err = tx.Exec(ctx, `UPDATE room_members SET display_name=$2 WHERE user_id=$1`, id, a.DisplayName); err != nil {
			return ManagedUser{}, err
		}
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "user." + change.Action, TargetType: "user", TargetID: id, Metadata: map[string]any{"reason": change.Reason}}); err != nil {
		return ManagedUser{}, err
	}
	return managedUser(a), tx.Commit(ctx)
}

const adminRoomColumns = `r.code,r.name,r.owner_id,r.source_url,COALESCE(r.media_source_id,''),COALESCE(r.media_path,''),r.max_members,r.created_at,r.expires_at,r.position,r.playing,r.speed,r.episode,r.position_ts,r.source_version,r.closed_at IS NOT NULL,r.admin_epoch,COALESCE(f.data,'{}'),COALESCE(f.version,0),u.display_name,u.email,COALESCE((SELECT jsonb_agg(jsonb_build_object('user_id',m.user_id,'display_name',m.display_name,'joined_at',m.joined_at) ORDER BY m.joined_at,m.user_id) FROM room_members m WHERE m.room_code=r.code),'[]')`
const adminRoomFrom = ` FROM rooms r JOIN users u ON u.id=r.owner_id LEFT JOIN room_features f ON f.room_code=r.code`

func scanAdminRoom(row pgx.Row) (AdminRoomRecord, error) {
	var item AdminRoomRecord
	room := &item.Room
	var raw, members []byte
	var version int64
	err := row.Scan(&room.Code, &room.Name, &room.OwnerID, &room.SourceURL, &room.MediaSourceID, &room.MediaPath, &room.MaxMembers, &room.CreatedAt, &room.ExpiresAt, &room.Playback.Position, &room.Playback.Playing, &room.Playback.Speed, &room.Playback.Episode, &room.Playback.PositionTS, &room.Playback.SourceVersion, &room.Closed, &room.AdminEpoch, &raw, &version, &item.OwnerName, &item.OwnerEmail, &members)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	if err != nil {
		return item, err
	}
	item.Features = defaultRoomFeatures()
	if err = json.Unmarshal(raw, &item.Features); err != nil {
		return item, err
	}
	item.Features.Version = version
	err = json.Unmarshal(members, &room.Members)
	room.Code = strings.TrimSpace(room.Code)
	return item, err
}
func (r *PostgresRepository) AdminFindRooms(ctx context.Context, q AdminQuery) ([]AdminRoomRecord, int, error) {
	where := ` WHERE ($1='' OR strpos(lower(r.code||' '||r.name||' '||u.email||' '||u.display_name),lower($1))>0) AND ($2='' OR (CASE WHEN COALESCE(f.data->>'admin_state','') NOT IN ('','active') THEN f.data->>'admin_state' WHEN r.closed_at IS NOT NULL THEN 'closed' WHEN r.expires_at<=$5 THEN 'expired' ELSE 'active' END)=$2) AND ($3='' OR r.owner_id=$3) AND ($4='' OR f.data->>'category'=$4)`
	args := []any{q.Search, q.Status, q.OwnerID, q.Category, time.Now().UnixMilli()}
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*)`+adminRoomFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+adminRoomColumns+adminRoomFrom+where+` ORDER BY r.created_at DESC,r.code LIMIT $6 OFFSET $7`, append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []AdminRoomRecord{}
	for rows.Next() {
		item, e := scanAdminRoom(rows)
		if e != nil {
			return nil, 0, e
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}
func (r *PostgresRepository) AdminReadRoom(ctx context.Context, code string) (AdminRoomRecord, error) {
	return scanAdminRoom(r.pool.QueryRow(ctx, `SELECT `+adminRoomColumns+adminRoomFrom+` WHERE r.code=$1`, code))
}
func (r *PostgresRepository) AdminSaveRoom(ctx context.Context, record AdminRoomRecord, expected int64, actor, action string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = postgresAdminActor(ctx, tx, actor, "room.manage"); err != nil {
		return err
	}
	current, err := scanAdminRoom(tx.QueryRow(ctx, `SELECT `+adminRoomColumns+adminRoomFrom+` WHERE r.code=$1 FOR UPDATE OF r`, record.Room.Code))
	if err != nil {
		return err
	}
	if current.Features.Version != expected || record.Features.Version != expected+1 {
		return ErrAdminConflict
	}
	if record.Room.OwnerID != current.Room.OwnerID {
		var active bool
		if err = tx.QueryRow(ctx, `SELECT account_status='active' FROM users WHERE id=$1`, record.Room.OwnerID).Scan(&active); err != nil {
			return err
		}
		if !active {
			return ErrForbidden
		}
	}
	raw, err := json.Marshal(record.Features)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `INSERT INTO room_features(room_code,version,data) VALUES($1,$3,$4) ON CONFLICT(room_code) DO UPDATE SET version=EXCLUDED.version,data=EXCLUDED.data WHERE room_features.version=$2`, record.Room.Code, expected, record.Features.Version, raw)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrAdminConflict
	}
	room := record.Room
	_, err = tx.Exec(ctx, `UPDATE rooms SET name=$2,max_members=$3,owner_id=$4,closed_at=CASE WHEN $5::boolean THEN COALESCE(closed_at,now()) ELSE NULL END,playing=CASE WHEN $5::boolean THEN false ELSE playing END,admin_epoch=$6 WHERE code=$1`, room.Code, room.Name, room.MaxMembers, room.OwnerID, room.Closed, record.Features.Version)
	if err != nil {
		return err
	}
	ids := []string{}
	for _, m := range room.Members {
		ids = append(ids, m.UserID)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM room_members WHERE room_code=$1 AND NOT(user_id=ANY($2))`, room.Code, ids); err != nil {
		return err
	}
	for _, m := range room.Members {
		_, err = tx.Exec(ctx, `INSERT INTO room_members(room_code,user_id,display_name,joined_at) VALUES($1,$2,$3,$4) ON CONFLICT(room_code,user_id) DO UPDATE SET display_name=EXCLUDED.display_name`, room.Code, m.UserID, m.DisplayName, m.JoinedAt)
		if err != nil {
			return err
		}
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "room.admin." + action, TargetType: "room", TargetID: room.Code, Metadata: map[string]any{"reason": record.Features.AdminReason}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const managedSourceColumns = `s.id,s.user_id,s.source_type,s.name,s.base_url,(extract(epoch FROM s.created_at)*1000)::bigint,(extract(epoch FROM s.updated_at)*1000)::bigint,s.disabled,s.admin_version,u.display_name,u.email`

func scanManagedSource(row pgx.Row) (ManagedSource, error) {
	var item ManagedSource
	err := row.Scan(&item.ID, &item.UserID, &item.Type, &item.Name, &item.BaseURL, &item.CreatedAt, &item.UpdatedAt, &item.Disabled, &item.AdminVersion, &item.OwnerName, &item.OwnerEmail)
	item.OwnerID = item.UserID
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	return item, err
}
func (r *PostgresRepository) AdminFindSources(ctx context.Context, q AdminQuery) ([]ManagedSource, int, error) {
	from := ` FROM media_sources s JOIN users u ON u.id=s.user_id WHERE ($1='' OR strpos(lower(s.name||' '||s.source_type||' '||u.email),lower($1))>0) AND ($2='' OR CASE WHEN s.disabled THEN 'disabled' ELSE 'enabled' END=$2) AND ($3='' OR s.user_id=$3) AND ($4='' OR s.source_type=$4)`
	args := []any{q.Search, q.Status, q.OwnerID, q.Kind}
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+managedSourceColumns+from+` ORDER BY s.created_at DESC,s.id LIMIT $5 OFFSET $6`, append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []ManagedSource{}
	for rows.Next() {
		v, e := scanManagedSource(rows)
		if e != nil {
			return nil, 0, e
		}
		items = append(items, v)
	}
	return items, total, rows.Err()
}
func (r *PostgresRepository) AdminMutateSource(ctx context.Context, id, actor string, change AdminSourceChange) (ManagedSource, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ManagedSource{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = postgresAdminActor(ctx, tx, actor, "source.manage"); err != nil {
		return ManagedSource{}, err
	}
	item, err := scanManagedSource(tx.QueryRow(ctx, `SELECT `+managedSourceColumns+` FROM media_sources s JOIN users u ON u.id=s.user_id WHERE s.id=$1 FOR UPDATE OF s`, id))
	if err != nil {
		return item, err
	}
	if item.AdminVersion != change.Version {
		return item, ErrAdminConflict
	}
	switch change.Action {
	case "rename":
		item.Name = change.Name
	case "enable":
		item.Disabled = false
	case "disable":
		item.Disabled = true
	case "delete":
	default:
		return item, ErrForbidden
	}
	if change.Action == "delete" {
		_, err = tx.Exec(ctx, `DELETE FROM media_sources WHERE id=$1`, id)
	} else {
		_, err = tx.Exec(ctx, `UPDATE media_sources SET name=$2,disabled=$3,admin_version=admin_version+1,updated_at=now() WHERE id=$1`, id, item.Name, item.Disabled)
	}
	if err != nil {
		return item, err
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "source." + change.Action, TargetType: "source", TargetID: id, Metadata: map[string]any{"reason": change.Reason}}); err != nil {
		return item, err
	}
	item.AdminVersion++
	item.UpdatedAt = time.Now().UnixMilli()
	return item, tx.Commit(ctx)
}
func (r *PostgresRepository) GetAdminDocument(ctx context.Context, key string) (AdminDocument, error) {
	d := AdminDocument{Key: key}
	err := r.pool.QueryRow(ctx, `SELECT version,data FROM admin_documents WHERE key=$1`, key).Scan(&d.Version, &d.Data)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, nil
	}
	return d, err
}
func (r *PostgresRepository) PutAdminDocument(ctx context.Context, d AdminDocument, actor string) (AdminDocument, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return d, err
	}
	defer tx.Rollback(ctx)
	if _, err = postgresAdminActor(ctx, tx, actor, adminDocumentPermission(d.Key)); err != nil {
		return d, err
	}
	var result pgconn.CommandTag
	if d.Version == 0 {
		result, err = tx.Exec(ctx, `INSERT INTO admin_documents(key,version,data,updated_by) VALUES($1,1,$2,$3) ON CONFLICT DO NOTHING`, d.Key, []byte(d.Data), actor)
	} else {
		result, err = tx.Exec(ctx, `UPDATE admin_documents SET data=$3,version=version+1,updated_by=$4,updated_at=now() WHERE key=$1 AND version=$2`, d.Key, d.Version, []byte(d.Data), actor)
	}
	if err != nil {
		return d, err
	}
	if result.RowsAffected() != 1 {
		return d, ErrAdminConflict
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "settings." + d.Key + ".update", TargetType: "settings", TargetID: d.Key}); err != nil {
		return d, err
	}
	d.Version++
	return d, tx.Commit(ctx)
}
