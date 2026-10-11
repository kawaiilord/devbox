package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

const approvalColumns = `a.id,a.kind,a.user_id,COALESCE(a.room_code,''),a.status,a.version,a.reason,a.created_at,a.reviewed_at,COALESCE(a.reviewed_by,''),u.display_name,u.email,COALESCE(r.name,'')`
const approvalFrom = ` FROM approval_requests a JOIN users u ON u.id=a.user_id LEFT JOIN rooms r ON r.code=a.room_code`

func scanApproval(row pgx.Row) (ApprovalRequest, error) {
	var a ApprovalRequest
	err := row.Scan(&a.ID, &a.Kind, &a.UserID, &a.RoomCode, &a.Status, &a.Version, &a.Reason, &a.CreatedAt, &a.ReviewedAt, &a.ReviewedBy, &a.DisplayName, &a.Email, &a.RoomName)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}
func insertApprovalTx(ctx context.Context, tx pgx.Tx, kind, userID, code string) (ApprovalRequest, error) {
	id := mustRandomString(16)
	_, err := tx.Exec(ctx, `INSERT INTO approval_requests(id,kind,user_id,room_code,created_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, id, kind, userID, nilIfEmpty(code), time.Now().UnixMilli())
	if err != nil {
		return ApprovalRequest{}, err
	}
	return scanApproval(tx.QueryRow(ctx, `SELECT `+approvalColumns+approvalFrom+` WHERE a.kind=$1 AND a.user_id=$2 AND COALESCE(a.room_code,'')=$3 AND a.status='pending'`, kind, userID, code))
}
func (r *PostgresRepository) RegisterAccount(ctx context.Context, a AccountRecord, review bool) (ApprovalRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ApprovalRequest{}, err
	}
	defer tx.Rollback(ctx)
	status := "active"
	if review {
		status = "pending"
	}
	_, err = tx.Exec(ctx, `INSERT INTO users(id,email,display_name,password_hash,created_at,is_admin,admin_role,account_status) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, a.ID, a.Email, a.DisplayName, a.PasswordHash, a.CreatedAt, a.IsAdmin, a.AdminRole, status)
	if err != nil {
		var p *pgconn.PgError
		if errors.As(err, &p) && p.Code == "23505" {
			return ApprovalRequest{}, ErrEmailExists
		}
		return ApprovalRequest{}, err
	}
	var item ApprovalRequest
	if review {
		item, err = insertApprovalTx(ctx, tx, "registration", a.ID, "")
		if err != nil {
			return item, err
		}
	}
	return item, tx.Commit(ctx)
}
func (r *PostgresRepository) CreateRoomWithApproval(ctx context.Context, room Room, f RoomFeatures, review bool) (ApprovalRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ApprovalRequest{}, err
	}
	defer tx.Rollback(ctx)
	var active bool
	if err = tx.QueryRow(ctx, `SELECT account_status='active' FROM users WHERE id=$1 FOR SHARE`, room.OwnerID).Scan(&active); err != nil {
		return ApprovalRequest{}, err
	}
	if !active {
		return ApprovalRequest{}, ErrForbidden
	}
	_, err = tx.Exec(ctx, `INSERT INTO rooms(code,name,owner_id,source_url,media_source_id,media_path,max_members,created_at,expires_at,position,playing,speed,episode,position_ts,source_version,closed_at,admin_epoch) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, room.Code, room.Name, room.OwnerID, room.SourceURL, nilIfEmpty(room.MediaSourceID), nilIfEmpty(room.MediaPath), room.MaxMembers, room.CreatedAt, room.ExpiresAt, room.Playback.Position, room.Playback.Playing, room.Playback.Speed, room.Playback.Episode, room.Playback.PositionTS, room.Playback.SourceVersion, closedAtValue(room.Closed), room.AdminEpoch)
	if err != nil {
		return ApprovalRequest{}, err
	}
	for _, m := range room.Members {
		if _, err = tx.Exec(ctx, `INSERT INTO room_members(room_code,user_id,display_name,joined_at) VALUES($1,$2,$3,$4)`, room.Code, m.UserID, m.DisplayName, m.JoinedAt); err != nil {
			return ApprovalRequest{}, err
		}
	}
	data, err := json.Marshal(f)
	if err != nil {
		return ApprovalRequest{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO room_features(room_code,version,data) VALUES($1,$2,$3)`, room.Code, f.Version, data); err != nil {
		return ApprovalRequest{}, err
	}
	var item ApprovalRequest
	if review {
		item, err = insertApprovalTx(ctx, tx, "room_creation", room.OwnerID, room.Code)
		if err != nil {
			return item, err
		}
	}
	return item, tx.Commit(ctx)
}
func (r *PostgresRepository) RequestRoomApproval(ctx context.Context, code, userID string) (ApprovalRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ApprovalRequest{}, err
	}
	defer tx.Rollback(ctx)
	record, err := scanAdminRoom(tx.QueryRow(ctx, `SELECT `+adminRoomColumns+adminRoomFrom+` WHERE r.code=$1 FOR SHARE OF r`, code))
	if err != nil {
		return ApprovalRequest{}, err
	}
	user, err := scanAccount(tx.QueryRow(ctx, `SELECT `+adminAccountColumns+` FROM users WHERE id=$1 FOR SHARE`, userID))
	if err != nil {
		return ApprovalRequest{}, err
	}
	if !accountActive(user.User) || record.Room.Closed || record.Room.ExpiresAt <= time.Now().UnixMilli() || record.Features.blocked(userID) {
		return ApprovalRequest{}, ErrForbidden
	}
	if !record.Features.RequireApproval {
		return ApprovalRequest{}, ErrAdminConflict
	}
	item, err := insertApprovalTx(ctx, tx, "room_join", userID, code)
	if err != nil {
		return item, err
	}
	return item, tx.Commit(ctx)
}
func (r *PostgresRepository) FindApprovals(ctx context.Context, actor, scope, code string, q AdminQuery) ([]ApprovalRequest, int, error) {
	a, err := r.UserByID(ctx, actor)
	if err != nil || (scope != "mine" && scope != "admin" && scope != "room") || !accountActive(a.User) {
		return nil, 0, ErrForbidden
	}
	if scope == "admin" && !permissionGranted(a.User, "review.manage") {
		return nil, 0, ErrForbidden
	}
	if scope == "room" && !permissionGranted(a.User, "review.manage") {
		var owner string
		if err = r.pool.QueryRow(ctx, `SELECT owner_id FROM rooms WHERE code=$1`, code).Scan(&owner); err != nil || owner != actor {
			return nil, 0, ErrForbidden
		}
	}
	where := ` WHERE ($1<>'mine' OR a.user_id=$2) AND ($1<>'room' OR (a.kind='room_join' AND a.room_code=$3)) AND ($4='' OR a.status=$4) AND ($5='' OR a.kind=$5) AND ($6='' OR strpos(lower(u.display_name||' '||u.email||' '||COALESCE(a.room_code,'')||' '||COALESCE(r.name,'')),lower($6))>0)`
	args := []any{scope, actor, code, q.Status, q.Kind, q.Search}
	var total int
	if err = r.pool.QueryRow(ctx, `SELECT count(*)`+approvalFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+approvalColumns+approvalFrom+where+` ORDER BY a.created_at DESC,a.id LIMIT $7 OFFSET $8`, append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []ApprovalRequest{}
	for rows.Next() {
		item, e := scanApproval(rows)
		if e != nil {
			return nil, 0, e
		}
		if scope == "room" {
			item.Email = ""
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}
func (r *PostgresRepository) DecideApproval(ctx context.Context, id, actor string, version int64, approve bool, reason string) (ApprovalRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ApprovalRequest{}, err
	}
	defer tx.Rollback(ctx)
	item, err := scanApproval(tx.QueryRow(ctx, `SELECT `+approvalColumns+approvalFrom+` WHERE a.id=$1 FOR UPDATE OF a`, id))
	if err != nil {
		return item, err
	}
	a, err := scanAccount(tx.QueryRow(ctx, `SELECT `+adminAccountColumns+` FROM users WHERE id=$1 FOR SHARE`, actor))
	if err != nil {
		return item, err
	}
	var record AdminRoomRecord
	if item.RoomCode != "" {
		record, err = scanAdminRoom(tx.QueryRow(ctx, `SELECT `+adminRoomColumns+adminRoomFrom+` WHERE r.code=$1 FOR UPDATE OF r`, item.RoomCode))
		if err != nil {
			return item, err
		}
	}
	if !approvalPermission(a.User, item, record.Room) {
		return item, ErrForbidden
	}
	if item.Version != version || item.Status != "pending" {
		return item, ErrAdminConflict
	}
	user, err := scanAccount(tx.QueryRow(ctx, `SELECT `+adminAccountColumns+` FROM users WHERE id=$1 FOR UPDATE`, item.UserID))
	if err != nil {
		return item, err
	}
	now := time.Now().UnixMilli()
	f := record.Features
	roomChanged := false
	switch item.Kind {
	case "registration":
		if user.AccountStatus != "pending" {
			if approve {
				return item, ErrAdminConflict
			}
			break
		}
		status := "rejected"
		if approve {
			status = "active"
		}
		_, err = tx.Exec(ctx, `UPDATE users SET account_status=$2,admin_version=admin_version+1,session_version=session_version+1 WHERE id=$1`, user.ID, status)
		if err != nil {
			return item, err
		}
	case "room_creation":
		if f.AdminState != "pending" {
			if approve {
				return item, ErrAdminConflict
			}
			break
		}
		if approve && (!accountActive(user.User) || record.Room.ExpiresAt <= now) {
			return item, ErrForbidden
		}
		f.AdminState = "rejected"
		if approve {
			f.AdminState = "active"
		}
		roomChanged = true
		f.Version++
		_, err = tx.Exec(ctx, `UPDATE rooms SET closed_at=CASE WHEN $2::boolean THEN NULL ELSE COALESCE(closed_at,now()) END,admin_epoch=$3 WHERE code=$1`, item.RoomCode, approve, f.Version)
		if err != nil {
			return item, err
		}
	case "room_join":
		if approve {
			if record.Room.Closed || record.Room.ExpiresAt <= now || f.blocked(user.ID) || !accountActive(user.User) {
				return item, ErrForbidden
			}
			member := false
			for _, m := range record.Room.Members {
				if m.UserID == user.ID {
					member = true
				}
			}
			if !member && len(record.Room.Members) >= record.Room.MaxMembers {
				return item, ErrRoomFull
			}
			_, err = tx.Exec(ctx, `INSERT INTO room_members(room_code,user_id,display_name,joined_at) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, item.RoomCode, user.ID, user.DisplayName, now)
			if err != nil {
				return item, err
			}
			roomChanged = true
			f.Version++
			if _, err = tx.Exec(ctx, `UPDATE rooms SET admin_epoch=$2 WHERE code=$1`, item.RoomCode, f.Version); err != nil {
				return item, err
			}
		}
	}
	if roomChanged {
		raw, e := json.Marshal(f)
		if e != nil {
			return item, e
		}
		_, err = tx.Exec(ctx, `UPDATE room_features SET version=$2,data=$3 WHERE room_code=$1`, item.RoomCode, f.Version, raw)
		if err != nil {
			return item, err
		}
	}
	item.Status = "rejected"
	if approve {
		item.Status = "approved"
	}
	item.Version++
	item.Reason = reason
	item.ReviewedAt = now
	item.ReviewedBy = actor
	if _, err = tx.Exec(ctx, `UPDATE approval_requests SET status=$2,version=$3,reason=$4,reviewed_at=$5,reviewed_by=$6 WHERE id=$1`, id, item.Status, item.Version, reason, now, actor); err != nil {
		return item, err
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "approval." + item.Status, TargetType: item.Kind, TargetID: item.ID, Metadata: map[string]any{"reason": reason}}); err != nil {
		return item, err
	}
	return item, tx.Commit(ctx)
}
