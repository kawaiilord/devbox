package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) CreateAccountDeletionRequest(ctx context.Context, userID, reason string) (AccountDeletionRequest, error) {
	account, err := r.UserByID(ctx, userID)
	if err != nil {
		return AccountDeletionRequest{}, err
	}
	if account.AdminRole != "" {
		return AccountDeletionRequest{}, ErrForbidden
	}
	emailHash := sha256.Sum256([]byte(strings.ToLower(account.Email)))
	userHash := sha256.Sum256([]byte(account.ID + ":" + strings.ToLower(account.Email)))
	item := AccountDeletionRequest{UserID: userID, Reason: reason, Status: "pending"}
	err = r.pool.QueryRow(ctx, `INSERT INTO account_deletion_requests(user_id,user_ref,email_hash,reason) VALUES($1,$2,$3,$4) RETURNING id,(extract(epoch FROM requested_at)*1000)::bigint`, userID, "deleted:"+hex.EncodeToString(userHash[:16]), hex.EncodeToString(emailHash[:]), reason).Scan(&item.ID, &item.RequestedAt)
	return item, err
}
func (r *PostgresRepository) GetAccountDeletionRequest(ctx context.Context, userID string) (AccountDeletionRequest, error) {
	return scanDeletion(r.pool.QueryRow(ctx, `SELECT id,COALESCE(user_id,''),reason,status,(extract(epoch FROM requested_at)*1000)::bigint,COALESCE(reviewed_by,''),COALESCE((extract(epoch FROM reviewed_at)*1000)::bigint,0),resolution,COALESCE((extract(epoch FROM executed_at)*1000)::bigint,0) FROM account_deletion_requests WHERE user_id=$1 ORDER BY id DESC LIMIT 1`, userID))
}
func scanDeletion(row pgx.Row) (AccountDeletionRequest, error) {
	var v AccountDeletionRequest
	err := row.Scan(&v.ID, &v.UserID, &v.Reason, &v.Status, &v.RequestedAt, &v.ReviewedBy, &v.ReviewedAt, &v.Resolution, &v.ExecutedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}
func (r *PostgresRepository) CancelAccountDeletionRequest(ctx context.Context, userID string) error {
	command, err := r.pool.Exec(ctx, `UPDATE account_deletion_requests SET status='cancelled' WHERE user_id=$1 AND status='pending'`, userID)
	if err == nil && command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return err
}
func (r *PostgresRepository) ListAccountDeletionRequests(ctx context.Context, status string, before int64, limit int) ([]AccountDeletionRequest, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,COALESCE(user_id,''),reason,status,(extract(epoch FROM requested_at)*1000)::bigint,COALESCE(reviewed_by,''),COALESCE((extract(epoch FROM reviewed_at)*1000)::bigint,0),resolution,COALESCE((extract(epoch FROM executed_at)*1000)::bigint,0) FROM account_deletion_requests WHERE ($1='' OR status=$1) AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT $3`, status, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AccountDeletionRequest{}
	for rows.Next() {
		v, err := scanDeletion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) ResolveAccountDeletionRequest(ctx context.Context, id int64, actor string, approve bool, resolution string) (AccountDeletionRequest, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return AccountDeletionRequest{}, err
	}
	defer tx.Rollback(ctx)
	var userID, userRef, status string
	err = tx.QueryRow(ctx, `SELECT COALESCE(user_id,''),user_ref,status FROM account_deletion_requests WHERE id=$1 FOR UPDATE`, id).Scan(&userID, &userRef, &status)
	if errors.Is(err, pgx.ErrNoRows) || status != "pending" {
		return AccountDeletionRequest{}, ErrNotFound
	}
	if err != nil {
		return AccountDeletionRequest{}, err
	}
	if !approve {
		_, err = tx.Exec(ctx, `UPDATE account_deletion_requests SET status='rejected',reviewed_by=$2,reviewed_at=now(),resolution=$3 WHERE id=$1`, id, actor, resolution)
		if err != nil {
			return AccountDeletionRequest{}, err
		}
		if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "account_deletion.reject", TargetType: "account_deletion", TargetID: strconvFormatCompat(id)}); err != nil {
			return AccountDeletionRequest{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return AccountDeletionRequest{}, err
		}
		return r.deletionByID(ctx, id)
	}
	var role string
	if err = tx.QueryRow(ctx, `SELECT admin_role FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&role); err != nil {
		return AccountDeletionRequest{}, err
	}
	if role != "" {
		return AccountDeletionRequest{}, ErrForbidden
	}
	if _, err = tx.Exec(ctx, `UPDATE payment_orders SET deleted_user_ref=$2,user_id=NULL WHERE user_id=$1`, userID, userRef); err != nil {
		return AccountDeletionRequest{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE activation_codes SET used_by=NULL WHERE used_by=$1`, userID); err != nil {
		return AccountDeletionRequest{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM room_bot_allowlist WHERE added_by=$1`, userID); err != nil {
		return AccountDeletionRequest{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM rooms WHERE owner_id=$1`, userID); err != nil {
		return AccountDeletionRequest{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE account_deletion_requests SET status='approved',reviewed_by=$2,reviewed_at=now(),resolution=$3,executed_at=now(),user_id=NULL WHERE id=$1`, id, actor, resolution); err != nil {
		return AccountDeletionRequest{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID); err != nil {
		return AccountDeletionRequest{}, err
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "account_deletion.execute", TargetType: "account_deletion", TargetID: strconvFormatCompat(id), Metadata: map[string]any{"user_ref": userRef}}); err != nil {
		return AccountDeletionRequest{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AccountDeletionRequest{}, err
	}
	result, err := r.deletionByID(ctx, id)
	result.UserID = userID
	return result, err
}
func (r *PostgresRepository) deletionByID(ctx context.Context, id int64) (AccountDeletionRequest, error) {
	return scanDeletion(r.pool.QueryRow(ctx, `SELECT id,COALESCE(user_id,''),reason,status,(extract(epoch FROM requested_at)*1000)::bigint,COALESCE(reviewed_by,''),COALESCE((extract(epoch FROM reviewed_at)*1000)::bigint,0),resolution,COALESCE((extract(epoch FROM executed_at)*1000)::bigint,0) FROM account_deletion_requests WHERE id=$1`, id))
}

func (r *PostgresRepository) CreateCopyrightComplaint(ctx context.Context, item CopyrightComplaint) (CopyrightComplaint, error) {
	raw, _ := json.Marshal(item.Evidence)
	err := r.pool.QueryRow(ctx, `INSERT INTO copyright_complaints(claimant_user_id,claimant_name,claimant_email,rights_basis,infringement_url,room_code,evidence,statement_accurate,signature_name) VALUES(NULLIF($1,''),$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9) RETURNING id,(extract(epoch FROM submitted_at)*1000)::bigint,(extract(epoch FROM due_at)*1000)::bigint,status`, item.ClaimantUserID, item.ClaimantName, item.ClaimantEmail, item.RightsBasis, item.InfringementURL, item.RoomCode, raw, item.StatementAccurate, item.SignatureName).Scan(&item.ID, &item.SubmittedAt, &item.DueAt, &item.Status)
	return item, err
}
func scanComplaint(row pgx.Row) (CopyrightComplaint, error) {
	var v CopyrightComplaint
	var raw []byte
	err := row.Scan(&v.ID, &v.ClaimantUserID, &v.ClaimantName, &v.ClaimantEmail, &v.RightsBasis, &v.InfringementURL, &v.RoomCode, &raw, &v.StatementAccurate, &v.SignatureName, &v.Status, &v.SubmittedAt, &v.DueAt, &v.ReviewedBy, &v.ReviewedAt, &v.Resolution)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &v.Evidence)
	}
	return v, err
}

const complaintColumns = `id,COALESCE(claimant_user_id,''),claimant_name,claimant_email,rights_basis,infringement_url,COALESCE(room_code,''),evidence,statement_accurate,signature_name,status,(extract(epoch FROM submitted_at)*1000)::bigint,(extract(epoch FROM due_at)*1000)::bigint,COALESCE(reviewed_by,''),COALESCE((extract(epoch FROM reviewed_at)*1000)::bigint,0),resolution`

func (r *PostgresRepository) ListCopyrightComplaints(ctx context.Context, status string, before int64, limit int) ([]CopyrightComplaint, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+complaintColumns+` FROM copyright_complaints WHERE ($1='' OR status=$1) AND ($2::bigint=0 OR id<$2) ORDER BY CASE WHEN status IN ('submitted','triaged') THEN 0 ELSE 1 END,due_at,id LIMIT $3`, status, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CopyrightComplaint{}
	for rows.Next() {
		v, err := scanComplaint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) ResolveCopyrightComplaint(ctx context.Context, id int64, actor, status, resolution string) (CopyrightComplaint, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CopyrightComplaint{}, err
	}
	defer tx.Rollback(ctx)
	var v CopyrightComplaint
	var raw []byte
	err = tx.QueryRow(ctx, `UPDATE copyright_complaints SET status=$2,resolution=$3,reviewed_by=$4,reviewed_at=now() WHERE id=$1 AND status IN ('submitted','triaged') RETURNING `+complaintColumns, id, status, resolution, actor).Scan(&v.ID, &v.ClaimantUserID, &v.ClaimantName, &v.ClaimantEmail, &v.RightsBasis, &v.InfringementURL, &v.RoomCode, &raw, &v.StatementAccurate, &v.SignatureName, &v.Status, &v.SubmittedAt, &v.DueAt, &v.ReviewedBy, &v.ReviewedAt, &v.Resolution)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	_ = json.Unmarshal(raw, &v.Evidence)
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "copyright_complaint.resolve", TargetType: "copyright_complaint", TargetID: strconvFormatCompat(id), Metadata: map[string]any{"status": status}}); err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}

var _ = time.Now
