package app

import (
	"context"
	"time"
)

func (r *PostgresRepository) StoreRegistrationInvites(ctx context.Context, actor string, codes []RegistrationInvite) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = postgresAdminActor(ctx, tx, actor, "config.write"); err != nil {
		return err
	}
	for _, c := range codes {
		if _, err = tx.Exec(ctx, `INSERT INTO registration_invites(id,code_hash,hint,batch_id,label,created_by,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, c.ID, c.CodeHash, c.Hint, c.BatchID, c.Label, actor, c.CreatedAt, c.ExpiresAt); err != nil {
			return err
		}
	}
	if len(codes) > 0 {
		if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "registration_invites.create", TargetType: "registration_invite_batch", TargetID: codes[0].BatchID, Metadata: map[string]any{"count": len(codes), "label": codes[0].Label}}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) FindRegistrationInvites(ctx context.Context, q AdminQuery) ([]RegistrationInvite, int, error) {
	now := time.Now().UnixMilli()
	state := `CASE WHEN used_at>0 THEN 'used' WHEN revoked_at>0 THEN 'revoked' WHEN expires_at>0 AND expires_at<=$1 THEN 'expired' ELSE 'active' END`
	filter := ` FROM registration_invites WHERE ($2='' OR (` + state + `)=$2) AND ($3='' OR strpos(lower(label||' '||batch_id||' '||hint),lower($3))>0)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*)`+filter, now, q.Status, q.Search).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id,hint,batch_id,label,COALESCE(created_by,''),created_at,expires_at,COALESCE(used_by,''),used_at,revoked_at,`+state+filter+` ORDER BY created_at DESC,id DESC LIMIT $4 OFFSET $5`, now, q.Status, q.Search, q.Limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []RegistrationInvite{}
	for rows.Next() {
		var c RegistrationInvite
		if err = rows.Scan(&c.ID, &c.Hint, &c.BatchID, &c.Label, &c.CreatedBy, &c.CreatedAt, &c.ExpiresAt, &c.UsedBy, &c.UsedAt, &c.RevokedAt, &c.Status); err != nil {
			return nil, 0, err
		}
		items = append(items, c)
	}
	return items, total, rows.Err()
}

func (r *PostgresRepository) RevokeRegistrationInvite(ctx context.Context, actor, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = postgresAdminActor(ctx, tx, actor, "config.write"); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE registration_invites SET revoked_at=$2 WHERE id=$1 AND used_at=0 AND revoked_at=0`, id, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrAdminConflict
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "registration_invites.revoke", TargetType: "registration_invite", TargetID: id}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
