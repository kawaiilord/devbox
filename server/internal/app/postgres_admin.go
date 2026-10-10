package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) GetRuntimeConfig(ctx context.Context) (RuntimeConfig, error) {
	config := RuntimeConfig{Features: map[string]bool{}}
	rows, err := r.pool.Query(ctx, `SELECT key,value,(extract(epoch FROM updated_at)*1000)::bigint FROM runtime_config WHERE key IN ('maintenance','features','branding')`)
	if err != nil {
		return config, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var raw []byte
		var updated int64
		if err := rows.Scan(&key, &raw, &updated); err != nil {
			return config, err
		}
		if updated > config.UpdatedAt {
			config.UpdatedAt = updated
		}
		switch key {
		case "maintenance":
			err = json.Unmarshal(raw, &config.Maintenance)
		case "features":
			err = json.Unmarshal(raw, &config.Features)
		case "branding":
			err = json.Unmarshal(raw, &config.Branding)
		}
		if err != nil {
			return config, err
		}
	}
	return config, rows.Err()
}

func (r *PostgresRepository) UpdateRuntimeConfig(ctx context.Context, config RuntimeConfig, actor string) (RuntimeConfig, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RuntimeConfig{}, err
	}
	defer tx.Rollback(ctx)
	values := map[string]any{"maintenance": config.Maintenance, "features": config.Features, "branding": config.Branding}
	for key, value := range values {
		raw, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return RuntimeConfig{}, marshalErr
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_config(key,value,updated_by,updated_at) VALUES($1,$2,$3,now()) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_by=EXCLUDED.updated_by,updated_at=now()`, key, raw, actor); err != nil {
			return RuntimeConfig{}, err
		}
	}
	if _, err := appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "config.update", TargetType: "runtime_config", TargetID: "global"}); err != nil {
		return RuntimeConfig{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RuntimeConfig{}, err
	}
	return r.GetRuntimeConfig(ctx)
}

func (r *PostgresRepository) ListAnnouncements(ctx context.Context, activeOnly bool, before int64, limit int) ([]Announcement, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,title,body,kind,active,COALESCE((extract(epoch FROM starts_at)*1000)::bigint,0),COALESCE((extract(epoch FROM ends_at)*1000)::bigint,0),created_by,(extract(epoch FROM created_at)*1000)::bigint FROM announcements WHERE ($1=false OR (active AND (starts_at IS NULL OR starts_at<=now()) AND (ends_at IS NULL OR ends_at>now()))) AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT $3`, activeOnly, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Announcement{}
	for rows.Next() {
		var item Announcement
		if err := rows.Scan(&item.ID, &item.Title, &item.Body, &item.Kind, &item.Active, &item.StartsAt, &item.EndsAt, &item.CreatedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CreateAnnouncement(ctx context.Context, item Announcement) (Announcement, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Announcement{}, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO announcements(title,body,kind,active,starts_at,ends_at,created_by) VALUES($1,$2,$3,$4,CASE WHEN $5::bigint=0 THEN NULL ELSE to_timestamp($5/1000.0) END,CASE WHEN $6::bigint=0 THEN NULL ELSE to_timestamp($6/1000.0) END,$7) RETURNING id,(extract(epoch FROM created_at)*1000)::bigint`, item.Title, item.Body, item.Kind, item.Active, item.StartsAt, item.EndsAt, item.CreatedBy).Scan(&item.ID, &item.CreatedAt)
	if err != nil {
		return Announcement{}, err
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: item.CreatedBy, Action: "announcement.create", TargetType: "announcement", TargetID: strconv.FormatInt(item.ID, 10)}); err != nil {
		return Announcement{}, err
	}
	return item, tx.Commit(ctx)
}

func (r *PostgresRepository) DeleteAnnouncement(ctx context.Context, id int64, actor string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `DELETE FROM announcements WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "announcement.delete", TargetType: "announcement", TargetID: strconv.FormatInt(id, 10)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) ListDeviceBans(ctx context.Context, before int64, limit int) ([]DeviceBan, error) {
	rows, err := r.pool.Query(ctx, `SELECT device_hash,COALESCE(ban_reason,''),(extract(epoch FROM banned_at)*1000)::bigint,COALESCE(banned_by,'') FROM devices WHERE banned_at IS NOT NULL AND ($1::bigint=0 OR banned_at<to_timestamp($1/1000.0)) ORDER BY banned_at DESC LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeviceBan{}
	for rows.Next() {
		var item DeviceBan
		if err := rows.Scan(&item.DeviceHash, &item.Reason, &item.BannedAt, &item.BannedBy); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UnbanDevice(ctx context.Context, hash, actor string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `UPDATE devices SET banned_at=NULL,ban_reason=NULL,banned_by=NULL,unbanned_at=now(),unbanned_by=$2 WHERE device_hash=$1 AND banned_at IS NOT NULL`, hash, actor)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err = tx.Exec(ctx, `UPDATE user_devices SET revoked_at=NULL,revoked_by_ban=false WHERE device_hash=$1 AND revoked_by_ban`, hash); err != nil {
		return err
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "device.unban", TargetType: "device", TargetID: hash}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) AdminDashboard(ctx context.Context) (AdminDashboard, error) {
	var value AdminDashboard
	err := r.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users),(SELECT count(*) FROM users WHERE email_verified_at IS NOT NULL),(SELECT count(*) FROM users WHERE vip_expires_at>now()),(SELECT count(*) FROM rooms),(SELECT count(*) FROM rooms WHERE closed_at IS NULL AND expires_at>(extract(epoch FROM now())*1000)::bigint),(SELECT count(*) FROM couples WHERE status='active'),(SELECT count(*) FROM reports WHERE status='pending'),(SELECT count(*) FROM payment_orders WHERE status='activated'),(SELECT COALESCE(sum(amount_minor),0) FROM payment_orders WHERE status='activated'),(SELECT count(*) FROM reviews),(SELECT count(*) FROM watch_records WHERE companion_count>0)`).Scan(&value.Users, &value.VerifiedUsers, &value.ActiveVIPUsers, &value.Rooms, &value.ActiveRooms, &value.Couples, &value.PendingReports, &value.ActivatedOrders, &value.RevenueMinor, &value.Reviews, &value.TogetherWatchings)
	return value, err
}

func (r *PostgresRepository) SearchAdminUsers(ctx context.Context, query string, before int64, limit int) ([]AdminUser, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,email,display_name,email_verified_at IS NOT NULL,admin_role,COALESCE((extract(epoch FROM vip_expires_at)*1000)::bigint,0),(extract(epoch FROM created_at)*1000)::bigint FROM users WHERE ($1='' OR email ILIKE '%'||$1||'%' OR display_name ILIKE '%'||$1||'%') AND ($2::bigint=0 OR created_at<to_timestamp($2/1000.0)) ORDER BY created_at DESC LIMIT $3`, query, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminUser{}
	for rows.Next() {
		var item AdminUser
		if err := rows.Scan(&item.ID, &item.Email, &item.DisplayName, &item.EmailVerified, &item.AdminRole, &item.VIPExpiresAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) SetAdminRole(ctx context.Context, userID, role, actor string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `UPDATE users SET admin_role=$2,is_admin=($2<>'') WHERE id=$1`, userID, role)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "user.role.update", TargetType: "user", TargetID: userID, Metadata: map[string]any{"role": role}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) AdminListOrders(ctx context.Context, status string, before int64, limit int) ([]Order, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+orderSelectColumns+` FROM payment_orders WHERE ($1='' OR status=$1) AND ($2::bigint=0 OR created_at<to_timestamp($2/1000.0)) ORDER BY created_at DESC LIMIT $3`, status, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Order{}
	for rows.Next() {
		item, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdateVIPPlan(ctx context.Context, plan VIPPlan, actor string) (VIPPlan, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return VIPPlan{}, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `UPDATE vip_plans SET title=$2,price_minor=$3,original_price_minor=$4,duration_days=$5,lifetime=$6,popular=$7,enabled=$8,updated_at=now() WHERE id=$1 RETURNING id,title,price_minor,original_price_minor,duration_days,lifetime,popular,enabled`, plan.ID, plan.Title, plan.PriceMinor, plan.OriginalPriceMinor, plan.DurationDays, plan.Lifetime, plan.Popular, plan.Enabled).Scan(&plan.ID, &plan.Title, &plan.PriceMinor, &plan.OriginalPriceMinor, &plan.DurationDays, &plan.Lifetime, &plan.Popular, &plan.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return VIPPlan{}, ErrNotFound
	}
	if err != nil {
		return VIPPlan{}, err
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "vip_plan.update", TargetType: "vip_plan", TargetID: plan.ID}); err != nil {
		return VIPPlan{}, err
	}
	return plan, tx.Commit(ctx)
}

func (r *PostgresRepository) GetRoomBotConfig(ctx context.Context) (RoomBotConfig, string, error) {
	var config RoomBotConfig
	var cipher string
	err := r.pool.QueryRow(ctx, `SELECT enabled,display_name,summon_policy,reply_policy,provider_base_url,model,credential_ciphertext,(extract(epoch FROM updated_at)*1000)::bigint FROM room_bot_config WHERE singleton`).Scan(&config.Enabled, &config.DisplayName, &config.SummonPolicy, &config.ReplyPolicy, &config.ProviderBaseURL, &config.Model, &cipher, &config.UpdatedAt)
	config.HasCredential = cipher != ""
	return config, cipher, err
}

func (r *PostgresRepository) UpdateRoomBotConfig(ctx context.Context, config RoomBotConfig, cipher, actor string) (RoomBotConfig, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RoomBotConfig{}, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO room_bot_config(singleton,enabled,display_name,summon_policy,reply_policy,provider_base_url,model,credential_ciphertext,updated_by,updated_at) VALUES(true,$1,$2,$3,$4,$5,$6,$7,$8,now()) ON CONFLICT(singleton) DO UPDATE SET enabled=EXCLUDED.enabled,display_name=EXCLUDED.display_name,summon_policy=EXCLUDED.summon_policy,reply_policy=EXCLUDED.reply_policy,provider_base_url=EXCLUDED.provider_base_url,model=EXCLUDED.model,credential_ciphertext=CASE WHEN EXCLUDED.credential_ciphertext='' THEN room_bot_config.credential_ciphertext ELSE EXCLUDED.credential_ciphertext END,updated_by=EXCLUDED.updated_by,updated_at=now() RETURNING credential_ciphertext<>'',(extract(epoch FROM updated_at)*1000)::bigint`, config.Enabled, config.DisplayName, config.SummonPolicy, config.ReplyPolicy, config.ProviderBaseURL, config.Model, cipher, actor).Scan(&config.HasCredential, &config.UpdatedAt)
	if err != nil {
		return RoomBotConfig{}, err
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "room_bot.update", TargetType: "room_bot", TargetID: "singleton"}); err != nil {
		return RoomBotConfig{}, err
	}
	config.Credential = ""
	return config, tx.Commit(ctx)
}
