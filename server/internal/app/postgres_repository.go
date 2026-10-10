package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_foundation.sql
var foundationMigration string

//go:embed migrations/002_account_security.sql
var accountSecurityMigration string

//go:embed migrations/003_media_sources.sql
var mediaSourcesMigration string

//go:embed migrations/004_room_chat.sql
var roomChatMigration string

//go:embed migrations/005_moderation.sql
var moderationMigration string

//go:embed migrations/006_emby_sources.sql
var embySourcesMigration string

//go:embed migrations/007_personal_library.sql
var personalLibraryMigration string

//go:embed migrations/008_danmaku.sql
var danmakuMigration string

//go:embed migrations/009_social_messaging.sql
var socialMessagingMigration string

//go:embed migrations/010_couple_space.sql
var coupleSpaceMigration string

//go:embed migrations/011_reviews.sql
var reviewsMigration string

//go:embed migrations/012_commerce.sql
var commerceMigration string

//go:embed migrations/013_admin_config.sql
var adminConfigMigration string

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func OpenPostgres(ctx context.Context, databaseURL string) (*PostgresRepository, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	config.MaxConns = 10
	config.MinIdleConns = 1
	config.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) Migrate(ctx context.Context) error {
	for _, migration := range []string{
		foundationMigration, accountSecurityMigration, mediaSourcesMigration, roomChatMigration,
		moderationMigration, embySourcesMigration, personalLibraryMigration, danmakuMigration,
		socialMessagingMigration,
		coupleSpaceMigration,
		reviewsMigration,
		commerceMigration,
		adminConfigMigration,
	} {
		if _, err := r.pool.Exec(ctx, migration); err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepository) CreateUser(ctx context.Context, account AccountRecord) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO users (id, email, display_name, password_hash, created_at, is_admin)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		account.ID,
		strings.ToLower(account.Email),
		account.DisplayName,
		account.PasswordHash,
		account.CreatedAt,
		account.IsAdmin,
	)
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return ErrEmailExists
	}
	return err
}

func (r *PostgresRepository) UserByEmail(ctx context.Context, email string) (AccountRecord, error) {
	return scanAccount(r.pool.QueryRow(
		ctx,
		`SELECT id, email, display_name, password_hash, created_at,
		 email_verified_at IS NOT NULL, session_version, is_admin, signature, admin_role,
		 COALESCE((extract(epoch FROM vip_expires_at) * 1000)::bigint, 0)
		 FROM users WHERE email = lower($1)`,
		email,
	))
}

func (r *PostgresRepository) UserByID(ctx context.Context, id string) (AccountRecord, error) {
	return scanAccount(r.pool.QueryRow(
		ctx,
		`SELECT id, email, display_name, password_hash, created_at,
		 email_verified_at IS NOT NULL, session_version, is_admin, signature, admin_role,
		 COALESCE((extract(epoch FROM vip_expires_at) * 1000)::bigint, 0)
		 FROM users WHERE id = $1`,
		id,
	))
}

func scanAccount(row pgx.Row) (AccountRecord, error) {
	var account AccountRecord
	err := row.Scan(
		&account.ID,
		&account.Email,
		&account.DisplayName,
		&account.PasswordHash,
		&account.CreatedAt,
		&account.EmailVerified,
		&account.SessionVersion,
		&account.IsAdmin,
		&account.Signature,
		&account.AdminRole,
		&account.VIPExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountRecord{}, ErrInvalidCredentials
	}
	return account, err
}

func (r *PostgresRepository) StoreRefreshToken(
	ctx context.Context,
	userID, deviceHash, hash string,
	expiresAt time.Time,
) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO refresh_tokens (token_hash, user_id, device_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		hash,
		userID,
		deviceHash,
		expiresAt,
	)
	return err
}

func (r *PostgresRepository) RotateRefreshToken(
	ctx context.Context,
	oldHash, expectedDeviceHash, newHash string,
	expiresAt time.Time,
) (RefreshIdentity, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RefreshIdentity{}, err
	}
	defer tx.Rollback(ctx)
	var userID, deviceHash string
	err = tx.QueryRow(
		ctx,
		`SELECT user_id, device_hash FROM refresh_tokens
		 WHERE token_hash = $1 AND device_hash = $2 AND revoked_at IS NULL AND expires_at > now()
		 FOR UPDATE`,
		oldHash,
		expectedDeviceHash,
	).Scan(&userID, &deviceHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefreshIdentity{}, ErrInvalidRefresh
	}
	if err != nil {
		return RefreshIdentity{}, err
	}
	if _, err = tx.Exec(
		ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1`,
		oldHash,
	); err != nil {
		return RefreshIdentity{}, err
	}
	if _, err = tx.Exec(
		ctx,
		`INSERT INTO refresh_tokens (token_hash, user_id, device_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		newHash,
		userID,
		deviceHash,
		expiresAt,
	); err != nil {
		return RefreshIdentity{}, err
	}
	account, err := scanAccount(tx.QueryRow(
		ctx,
		`SELECT id, email, display_name, password_hash, created_at,
		 email_verified_at IS NOT NULL, session_version, is_admin, signature, admin_role,
		 COALESCE((extract(epoch FROM vip_expires_at) * 1000)::bigint, 0) FROM users WHERE id = $1`,
		userID,
	))
	if err != nil {
		return RefreshIdentity{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RefreshIdentity{}, err
	}
	return RefreshIdentity{Account: account, DeviceHash: deviceHash}, nil
}

func (r *PostgresRepository) RevokeRefreshToken(ctx context.Context, hash string) error {
	_, err := r.pool.Exec(
		ctx,
		`UPDATE refresh_tokens SET revoked_at = COALESCE(revoked_at, now()) WHERE token_hash = $1`,
		hash,
	)
	return err
}

func (r *PostgresRepository) RevokeUserRefreshTokens(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(
		ctx,
		`UPDATE refresh_tokens SET revoked_at = COALESCE(revoked_at, now())
		 WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	)
	return err
}

func (r *PostgresRepository) BindDevice(
	ctx context.Context,
	userID, deviceHash string,
	info DeviceInfo,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var banned bool
	err = tx.QueryRow(
		ctx,
		`INSERT INTO devices (device_hash, label, platform)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (device_hash) DO UPDATE SET
		   label = EXCLUDED.label, platform = EXCLUDED.platform, last_seen = now()
		 RETURNING banned_at IS NOT NULL`,
		deviceHash,
		info.Label,
		info.Platform,
	).Scan(&banned)
	if err != nil {
		return err
	}
	if banned {
		return ErrDeviceBlocked
	}
	var revoked bool
	err = tx.QueryRow(
		ctx,
		`SELECT revoked_at IS NOT NULL FROM user_devices
		 WHERE user_id = $1 AND device_hash = $2`,
		userID,
		deviceHash,
	).Scan(&revoked)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if revoked {
		return ErrDeviceBlocked
	}
	_, err = tx.Exec(
		ctx,
		`INSERT INTO user_devices (user_id, device_hash)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id, device_hash) DO UPDATE SET last_seen = now()`,
		userID,
		deviceHash,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) ValidateDevice(ctx context.Context, userID, deviceHash string) error {
	command, err := r.pool.Exec(
		ctx,
		`UPDATE user_devices ud SET last_seen = now()
		 FROM devices d
		 WHERE ud.user_id = $1 AND ud.device_hash = $2
		   AND ud.device_hash = d.device_hash
		   AND ud.revoked_at IS NULL AND d.banned_at IS NULL`,
		userID,
		deviceHash,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrDeviceBlocked
	}
	_, _ = r.pool.Exec(ctx, `UPDATE devices SET last_seen = now() WHERE device_hash = $1`, deviceHash)
	return nil
}

func (r *PostgresRepository) UserDevices(ctx context.Context, userID string) ([]UserDevice, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT d.device_hash, d.label, d.platform,
		 (extract(epoch FROM ud.last_seen) * 1000)::bigint,
		 (extract(epoch FROM ud.linked_at) * 1000)::bigint
		 FROM user_devices ud JOIN devices d ON d.device_hash = ud.device_hash
		 WHERE ud.user_id = $1 AND ud.revoked_at IS NULL AND d.banned_at IS NULL
		 ORDER BY ud.last_seen DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := make([]UserDevice, 0)
	for rows.Next() {
		var device UserDevice
		if err := rows.Scan(&device.ID, &device.Label, &device.Platform, &device.LastSeen, &device.CreatedAt); err != nil {
			return nil, err
		}
		device.ID = strings.TrimSpace(device.ID)
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (r *PostgresRepository) RevokeUserDevice(ctx context.Context, userID, deviceHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(
		ctx,
		`UPDATE user_devices SET revoked_at = now()
		 WHERE user_id = $1 AND device_hash = $2 AND revoked_at IS NULL`,
		userID,
		deviceHash,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	_, err = tx.Exec(
		ctx,
		`UPDATE refresh_tokens SET revoked_at = COALESCE(revoked_at, now())
		 WHERE user_id = $1 AND device_hash = $2 AND revoked_at IS NULL`,
		userID,
		deviceHash,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) StoreActionToken(ctx context.Context, record ActionTokenRecord) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(
		ctx,
		`UPDATE auth_action_tokens SET used_at = now()
		 WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL`,
		record.UserID,
		record.Purpose,
	); err != nil {
		return err
	}
	if _, err = tx.Exec(
		ctx,
		`INSERT INTO auth_action_tokens (token_hash, user_id, purpose, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		record.Hash,
		record.UserID,
		record.Purpose,
		record.ExpiresAt,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) ConsumeActionToken(
	ctx context.Context,
	hash, purpose string,
) (AccountRecord, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return AccountRecord{}, err
	}
	defer tx.Rollback(ctx)
	var userID string
	err = tx.QueryRow(
		ctx,
		`UPDATE auth_action_tokens SET used_at = now()
		 WHERE token_hash = $1 AND purpose = $2
		   AND used_at IS NULL AND expires_at > now()
		 RETURNING user_id`,
		hash,
		purpose,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountRecord{}, ErrInvalidActionToken
	}
	if err != nil {
		return AccountRecord{}, err
	}
	account, err := scanAccount(tx.QueryRow(
		ctx,
		`SELECT id, email, display_name, password_hash, created_at,
		 email_verified_at IS NOT NULL, session_version, is_admin, signature, admin_role,
		 COALESCE((extract(epoch FROM vip_expires_at) * 1000)::bigint, 0) FROM users WHERE id = $1`,
		userID,
	))
	if err != nil {
		return AccountRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AccountRecord{}, err
	}
	return account, nil
}

func (r *PostgresRepository) VerifyEmail(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(
		ctx,
		`UPDATE users SET email_verified_at = COALESCE(email_verified_at, now()) WHERE id = $1`,
		userID,
	)
	return err
}

func (r *PostgresRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	_, err := r.pool.Exec(
		ctx,
		`UPDATE users SET password_hash = $2, session_version = session_version + 1 WHERE id = $1`,
		userID,
		passwordHash,
	)
	return err
}

func (r *PostgresRepository) CreateMediaSource(ctx context.Context, source MediaSource) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO media_sources (
		 id, user_id, source_type, name, base_url, credentials_ciphertext, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,to_timestamp($7 / 1000.0),to_timestamp($8 / 1000.0))`,
		source.ID,
		source.UserID,
		source.Type,
		source.Name,
		source.BaseURL,
		source.CredentialsCiphertext,
		source.CreatedAt,
		source.UpdatedAt,
	)
	return err
}

func (r *PostgresRepository) ListMediaSources(ctx context.Context, userID string) ([]MediaSource, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT id, user_id, source_type, name, base_url, credentials_ciphertext,
		 (extract(epoch FROM created_at) * 1000)::bigint,
		 (extract(epoch FROM updated_at) * 1000)::bigint
		 FROM media_sources WHERE user_id = $1 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := make([]MediaSource, 0)
	for rows.Next() {
		var source MediaSource
		if err := rows.Scan(
			&source.ID,
			&source.UserID,
			&source.Type,
			&source.Name,
			&source.BaseURL,
			&source.CredentialsCiphertext,
			&source.CreatedAt,
			&source.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (r *PostgresRepository) GetMediaSource(
	ctx context.Context,
	userID, sourceID string,
) (MediaSource, error) {
	var source MediaSource
	err := r.pool.QueryRow(
		ctx,
		`SELECT id, user_id, source_type, name, base_url, credentials_ciphertext,
		 (extract(epoch FROM created_at) * 1000)::bigint,
		 (extract(epoch FROM updated_at) * 1000)::bigint
		 FROM media_sources WHERE id = $1 AND user_id = $2`,
		sourceID,
		userID,
	).Scan(
		&source.ID,
		&source.UserID,
		&source.Type,
		&source.Name,
		&source.BaseURL,
		&source.CredentialsCiphertext,
		&source.CreatedAt,
		&source.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaSource{}, ErrNotFound
	}
	return source, err
}

func (r *PostgresRepository) DeleteMediaSource(ctx context.Context, userID, sourceID string) error {
	command, err := r.pool.Exec(
		ctx,
		`DELETE FROM media_sources WHERE id = $1 AND user_id = $2`,
		sourceID,
		userID,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpsertFavorite(ctx context.Context, favorite Favorite) (Favorite, error) {
	source, err := r.GetMediaSource(ctx, favorite.UserID, favorite.SourceID)
	if err != nil {
		return Favorite{}, err
	}
	err = r.pool.QueryRow(
		ctx,
		`INSERT INTO favorites (
		   user_id, media_source_id, media_path, title, content_type, size, updated_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,now())
		 ON CONFLICT (user_id, media_source_id, media_path) DO UPDATE SET
		   title=EXCLUDED.title, content_type=EXCLUDED.content_type,
		   size=EXCLUDED.size, updated_at=now()
		 RETURNING id, (extract(epoch FROM updated_at) * 1000)::bigint`,
		favorite.UserID,
		favorite.SourceID,
		favorite.MediaPath,
		favorite.Title,
		favorite.ContentType,
		favorite.Size,
	).Scan(&favorite.ID, &favorite.UpdatedAt)
	if err != nil {
		return Favorite{}, err
	}
	favorite.SourceType = source.Type
	favorite.SourceName = source.Name
	return favorite, nil
}

func (r *PostgresRepository) ListFavorites(
	ctx context.Context,
	userID string,
	before int64,
	limit int,
) ([]Favorite, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT f.id, f.user_id, f.media_source_id, s.source_type, s.name,
		        f.media_path, f.title, f.content_type, f.size,
		        (extract(epoch FROM f.updated_at) * 1000)::bigint
		 FROM favorites f JOIN media_sources s ON s.id = f.media_source_id
		 WHERE f.user_id=$1
		   AND ($2::bigint=0 OR (extract(epoch FROM f.updated_at) * 1000)::bigint < $2)
		 ORDER BY f.updated_at DESC, f.id DESC LIMIT $3`,
		userID,
		before,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Favorite, 0, limit)
	for rows.Next() {
		var favorite Favorite
		if err := rows.Scan(
			&favorite.ID, &favorite.UserID, &favorite.SourceID,
			&favorite.SourceType, &favorite.SourceName, &favorite.MediaPath,
			&favorite.Title, &favorite.ContentType, &favorite.Size, &favorite.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, favorite)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) DeleteFavorite(ctx context.Context, userID string, id int64) error {
	command, err := r.pool.Exec(ctx, `DELETE FROM favorites WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpsertWatchRecord(ctx context.Context, record WatchRecord) (WatchRecord, error) {
	if record.SourceID != "" {
		if _, err := r.GetMediaSource(ctx, record.UserID, record.SourceID); err != nil {
			record.SourceID = ""
			record.MediaPath = ""
		}
	}
	err := r.pool.QueryRow(
		ctx,
		`INSERT INTO watch_records (
		   user_id, media_key, media_source_id, media_path, title,
		   position_seconds, duration_seconds, episode, completed,
		   companion_count, room_code, watched_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now())
		 ON CONFLICT (user_id, media_key) DO UPDATE SET
		   media_source_id=EXCLUDED.media_source_id, media_path=EXCLUDED.media_path,
		   title=EXCLUDED.title, position_seconds=EXCLUDED.position_seconds,
		   duration_seconds=EXCLUDED.duration_seconds, episode=EXCLUDED.episode,
		   completed=watch_records.completed OR EXCLUDED.completed,
		   companion_count=GREATEST(watch_records.companion_count, EXCLUDED.companion_count),
		   room_code=EXCLUDED.room_code, watched_at=now()
		 RETURNING id, completed, companion_count,
		           (extract(epoch FROM watched_at) * 1000)::bigint`,
		record.UserID,
		record.MediaKey,
		nilIfEmpty(record.SourceID),
		record.MediaPath,
		record.Title,
		record.Position,
		record.Duration,
		record.Episode,
		record.Completed,
		record.CompanionCount,
		nilIfEmpty(record.RoomCode),
	).Scan(&record.ID, &record.Completed, &record.CompanionCount, &record.WatchedAt)
	if err != nil {
		return WatchRecord{}, err
	}
	if record.SourceID != "" {
		_, err := r.GetMediaSource(ctx, record.UserID, record.SourceID)
		record.Resumable = err == nil
	}
	return record, nil
}

func (r *PostgresRepository) ListWatchRecords(
	ctx context.Context,
	userID string,
	before int64,
	limit int,
) ([]WatchRecord, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT w.id, w.user_id, w.media_key, COALESCE(w.media_source_id, ''),
		        w.media_path, w.title, w.position_seconds, w.duration_seconds,
		        w.episode, w.completed, w.companion_count,
		        COALESCE(w.room_code::text, ''),
		        EXISTS (
		          SELECT 1 FROM media_sources s
		          WHERE s.id=w.media_source_id AND s.user_id=w.user_id
		        ),
		        (extract(epoch FROM w.watched_at) * 1000)::bigint
		 FROM watch_records w
		 WHERE w.user_id=$1
		   AND ($2::bigint=0 OR (extract(epoch FROM w.watched_at) * 1000)::bigint < $2)
		 ORDER BY w.watched_at DESC, w.id DESC LIMIT $3`,
		userID,
		before,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]WatchRecord, 0, limit)
	for rows.Next() {
		var record WatchRecord
		if err := rows.Scan(
			&record.ID, &record.UserID, &record.MediaKey, &record.SourceID,
			&record.MediaPath, &record.Title, &record.Position, &record.Duration,
			&record.Episode, &record.Completed, &record.CompanionCount,
			&record.RoomCode, &record.Resumable, &record.WatchedAt,
		); err != nil {
			return nil, err
		}
		record.RoomCode = strings.TrimSpace(record.RoomCode)
		if record.SourceID == "" {
			record.MediaPath = ""
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) DeleteWatchRecord(ctx context.Context, userID string, id int64) error {
	command, err := r.pool.Exec(ctx, `DELETE FROM watch_records WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) AddDanmaku(
	ctx context.Context,
	message DanmakuMessage,
) (DanmakuMessage, error) {
	err := r.pool.QueryRow(
		ctx,
		`INSERT INTO danmaku_messages (
		   media_fingerprint, user_id, display_name, body,
		   position_seconds, color, mode
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7)
		 RETURNING id, (extract(epoch FROM created_at) * 1000)::bigint`,
		message.Fingerprint,
		message.UserID,
		message.DisplayName,
		message.Body,
		message.Position,
		message.Color,
		message.Mode,
	).Scan(&message.ID, &message.CreatedAt)
	return message, err
}

func (r *PostgresRepository) ListDanmaku(
	ctx context.Context,
	fingerprint string,
	viewerID string,
	from, to float64,
	limit int,
) ([]DanmakuMessage, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT id, media_fingerprint, user_id, display_name, body,
		        position_seconds, color, mode,
		        (extract(epoch FROM created_at) * 1000)::bigint
		 FROM danmaku_messages
		 WHERE media_fingerprint=$1 AND position_seconds >= $3 AND position_seconds <= $4
		   AND NOT EXISTS (
		     SELECT 1 FROM user_blocks b
		     WHERE (b.blocker_id=$2 AND b.blocked_id=danmaku_messages.user_id)
		        OR (b.blocker_id=danmaku_messages.user_id AND b.blocked_id=$2)
		   )
		 ORDER BY position_seconds, id LIMIT $5`,
		fingerprint,
		viewerID,
		from,
		to,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]DanmakuMessage, 0, limit)
	for rows.Next() {
		var message DanmakuMessage
		if err := rows.Scan(
			&message.ID, &message.Fingerprint, &message.UserID,
			&message.DisplayName, &message.Body, &message.Position,
			&message.Color, &message.Mode, &message.CreatedAt,
		); err != nil {
			return nil, err
		}
		message.Fingerprint = strings.TrimSpace(message.Fingerprint)
		result = append(result, message)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) AddRoomMessage(
	ctx context.Context,
	message ChatMessage,
) (ChatMessage, error) {
	err := r.pool.QueryRow(
		ctx,
		`INSERT INTO room_messages (room_code, user_id, display_name, body)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id, (extract(epoch FROM created_at) * 1000)::bigint`,
		message.RoomCode,
		message.UserID,
		message.DisplayName,
		message.Body,
	).Scan(&message.ID, &message.CreatedAt)
	return message, err
}

func (r *PostgresRepository) ListRoomMessages(
	ctx context.Context,
	roomCode string,
	viewerID string,
	before int64,
	limit int,
) ([]ChatMessage, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT id, room_code, user_id, display_name, body,
		 (extract(epoch FROM created_at) * 1000)::bigint
		 FROM room_messages m
		 WHERE room_code = $1 AND ($3::bigint = 0 OR id < $3)
		   AND NOT EXISTS (
		     SELECT 1 FROM user_blocks b
		     WHERE (b.blocker_id = $2 AND b.blocked_id = m.user_id)
		        OR (b.blocker_id = m.user_id AND b.blocked_id = $2)
		   )
		 ORDER BY id DESC LIMIT $4`,
		roomCode,
		viewerID,
		before,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]ChatMessage, 0, limit)
	for rows.Next() {
		var message ChatMessage
		if err := rows.Scan(
			&message.ID,
			&message.RoomCode,
			&message.UserID,
			&message.DisplayName,
			&message.Body,
			&message.CreatedAt,
		); err != nil {
			return nil, err
		}
		message.RoomCode = strings.TrimSpace(message.RoomCode)
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return messages, nil
}

func (r *PostgresRepository) GetPrivacy(ctx context.Context, userID string) (PrivacySettings, error) {
	settings := PrivacySettings{
		AllowRoomChat: true, AllowPrivateChat: true,
		AllowProfileFind: true, ShowWatchActivity: true,
	}
	err := r.pool.QueryRow(
		ctx,
		`SELECT allow_room_chat, allow_private_chat, allow_profile_find, show_watch_activity
		 FROM user_privacy WHERE user_id = $1`,
		userID,
	).Scan(
		&settings.AllowRoomChat, &settings.AllowPrivateChat,
		&settings.AllowProfileFind, &settings.ShowWatchActivity,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return settings, nil
	}
	return settings, err
}

func (r *PostgresRepository) UpdatePrivacy(
	ctx context.Context,
	userID string,
	settings PrivacySettings,
) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO user_privacy (
		   user_id, allow_room_chat, allow_private_chat,
		   allow_profile_find, show_watch_activity, updated_at
		 ) VALUES ($1,$2,$3,$4,$5,now())
		 ON CONFLICT (user_id) DO UPDATE SET
		   allow_room_chat = EXCLUDED.allow_room_chat,
		   allow_private_chat = EXCLUDED.allow_private_chat,
		   allow_profile_find = EXCLUDED.allow_profile_find,
		   show_watch_activity = EXCLUDED.show_watch_activity,
		   updated_at = now()`,
		userID,
		settings.AllowRoomChat,
		settings.AllowPrivateChat,
		settings.AllowProfileFind,
		settings.ShowWatchActivity,
	)
	return err
}

func (r *PostgresRepository) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	if blockerID == blockedID {
		return errors.New("cannot block self")
	}
	command, err := r.pool.Exec(
		ctx,
		`INSERT INTO user_blocks (blocker_id, blocked_id)
		 SELECT $1, id FROM users WHERE id = $2
		 ON CONFLICT (blocker_id, blocked_id) DO UPDATE SET blocker_id = EXCLUDED.blocker_id`,
		blockerID,
		blockedID,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	_, err := r.pool.Exec(
		ctx,
		`DELETE FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2`,
		blockerID,
		blockedID,
	)
	return err
}

func (r *PostgresRepository) ListBlockedUsers(ctx context.Context, userID string) ([]User, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT u.id, u.display_name
		 FROM user_blocks b JOIN users u ON u.id = b.blocked_id
		 WHERE b.blocker_id = $1 ORDER BY lower(u.display_name), u.id`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.DisplayName); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (r *PostgresRepository) UsersBlocked(ctx context.Context, userA, userB string) (bool, error) {
	var blocked bool
	err := r.pool.QueryRow(
		ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM user_blocks
		   WHERE (blocker_id = $1 AND blocked_id = $2)
		      OR (blocker_id = $2 AND blocked_id = $1)
		 )`,
		userA,
		userB,
	).Scan(&blocked)
	return blocked, err
}

func (r *PostgresRepository) CreateReport(ctx context.Context, report Report) (Report, error) {
	err := r.pool.QueryRow(
		ctx,
		`INSERT INTO reports (reporter_id, target_type, target_id, reason, details)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, status, (extract(epoch FROM created_at) * 1000)::bigint`,
		report.ReporterID,
		report.TargetType,
		report.TargetID,
		report.Reason,
		report.Details,
	).Scan(&report.ID, &report.Status, &report.CreatedAt)
	return report, err
}

func (r *PostgresRepository) ListReports(
	ctx context.Context,
	status string,
	before int64,
	limit int,
) ([]Report, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT id, reporter_id, target_type, target_id, reason, details, status,
		        resolution, (extract(epoch FROM created_at) * 1000)::bigint,
		        COALESCE(reviewed_by, ''),
		        COALESCE((extract(epoch FROM reviewed_at) * 1000)::bigint, 0)
		 FROM reports
		 WHERE ($1 = '' OR status = $1) AND ($2::bigint = 0 OR id < $2)
		 ORDER BY id DESC LIMIT $3`,
		status,
		before,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reports := make([]Report, 0, limit)
	for rows.Next() {
		var report Report
		if err := rows.Scan(
			&report.ID, &report.ReporterID, &report.TargetType, &report.TargetID,
			&report.Reason, &report.Details, &report.Status, &report.Resolution,
			&report.CreatedAt, &report.ReviewedBy, &report.ReviewedAt,
		); err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func (r *PostgresRepository) ResolveReport(
	ctx context.Context,
	id int64,
	reviewerID, status, resolution string,
) (Report, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback(ctx)
	var report Report
	err = tx.QueryRow(
		ctx,
		`UPDATE reports SET status=$3, resolution=$4, reviewed_by=$2, reviewed_at=now()
		 WHERE id=$1
		 RETURNING id, reporter_id, target_type, target_id, reason, details, status,
		           resolution, (extract(epoch FROM created_at) * 1000)::bigint,
		           reviewed_by, (extract(epoch FROM reviewed_at) * 1000)::bigint`,
		id,
		reviewerID,
		status,
		resolution,
	).Scan(
		&report.ID, &report.ReporterID, &report.TargetType, &report.TargetID,
		&report.Reason, &report.Details, &report.Status, &report.Resolution,
		&report.CreatedAt, &report.ReviewedBy, &report.ReviewedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	if err != nil {
		return Report{}, err
	}
	if _, err := appendPostgresAudit(ctx, tx, AuditEvent{
		ActorID: reviewerID, Action: "report.resolve", TargetType: "report",
		TargetID: strconv.FormatInt(id, 10), Metadata: map[string]any{"status": status},
	}); err != nil {
		return Report{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Report{}, err
	}
	return report, nil
}

func (r *PostgresRepository) CloseRoom(ctx context.Context, code, actorID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(
		ctx,
		`UPDATE rooms SET closed_at = COALESCE(closed_at, now()) WHERE code = $1`,
		strings.ToUpper(code),
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := appendPostgresAudit(ctx, tx, AuditEvent{
		ActorID: actorID, Action: "room.close", TargetType: "room", TargetID: strings.ToUpper(code),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) BanDevice(ctx context.Context, deviceHash, reason, actorID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(
		ctx,
		`UPDATE devices SET banned_at = COALESCE(banned_at, now()), ban_reason = $2
		 WHERE device_hash = $1`,
		deviceHash,
		reason,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE user_devices SET revoked_at = COALESCE(revoked_at, now())
		 WHERE device_hash = $1`,
		deviceHash,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE refresh_tokens SET revoked_at = COALESCE(revoked_at, now())
		 WHERE device_hash = $1`,
		deviceHash,
	); err != nil {
		return err
	}
	if _, err := appendPostgresAudit(ctx, tx, AuditEvent{
		ActorID: actorID, Action: "device.ban", TargetType: "device", TargetID: deviceHash,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) SetUserAdmin(ctx context.Context, userID string, value bool) error {
	command, err := r.pool.Exec(ctx, `UPDATE users SET is_admin=$2,admin_role=CASE WHEN $2 THEN 'super_admin' ELSE '' END WHERE id=$1`, userID, value)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SetUserVIP(ctx context.Context, userID string, expiresAt int64) error {
	command, err := r.pool.Exec(ctx, `UPDATE users SET vip_expires_at=CASE WHEN $2::bigint=0 THEN NULL ELSE to_timestamp($2/1000.0) END WHERE id=$1`, userID, expiresAt)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) AppendAudit(ctx context.Context, event AuditEvent) (AuditEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AuditEvent{}, err
	}
	defer tx.Rollback(ctx)
	event, err = appendPostgresAudit(ctx, tx, event)
	if err != nil {
		return AuditEvent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AuditEvent{}, err
	}
	return event, nil
}

func appendPostgresAudit(ctx context.Context, tx pgx.Tx, event AuditEvent) (AuditEvent, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('sameframe-admin-audit'))`); err != nil {
		return AuditEvent{}, err
	}
	err := tx.QueryRow(
		ctx,
		`SELECT entry_hash FROM admin_audit_logs ORDER BY id DESC LIMIT 1`,
	).Scan(&event.PreviousHash)
	if errors.Is(err, pgx.ErrNoRows) {
		event.PreviousHash = strings.Repeat("0", 64)
	} else if err != nil {
		return AuditEvent{}, err
	}
	event.CreatedAt = time.Now().UnixMilli()
	event = finalizeAuditEvent(event)
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return AuditEvent{}, err
	}
	err = tx.QueryRow(
		ctx,
		`INSERT INTO admin_audit_logs (
		   actor_id, action, target_type, target_id, metadata,
		   previous_hash, entry_hash, created_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,to_timestamp($8 / 1000.0)) RETURNING id`,
		event.ActorID,
		event.Action,
		event.TargetType,
		event.TargetID,
		metadata,
		event.PreviousHash,
		event.EntryHash,
		event.CreatedAt,
	).Scan(&event.ID)
	if err != nil {
		return AuditEvent{}, err
	}
	return event, nil
}

func (r *PostgresRepository) ListAudit(ctx context.Context, before int64, limit int) ([]AuditEvent, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT id, actor_id, action, target_type, target_id, metadata,
		        previous_hash, entry_hash, (extract(epoch FROM created_at) * 1000)::bigint
		 FROM admin_audit_logs
		 WHERE ($1::bigint = 0 OR id < $1) ORDER BY id DESC LIMIT $2`,
		before,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]AuditEvent, 0, limit)
	for rows.Next() {
		var event AuditEvent
		var metadata []byte
		if err := rows.Scan(
			&event.ID, &event.ActorID, &event.Action, &event.TargetType,
			&event.TargetID, &metadata, &event.PreviousHash, &event.EntryHash,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
			return nil, err
		}
		event.PreviousHash = strings.TrimSpace(event.PreviousHash)
		event.EntryHash = strings.TrimSpace(event.EntryHash)
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *PostgresRepository) SaveRoom(ctx context.Context, room Room) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(
		ctx,
		`INSERT INTO rooms (
			code, name, owner_id, source_url, media_source_id, media_path,
			max_members, created_at, expires_at,
			position, playing, speed, episode, position_ts, source_version, closed_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		 ON CONFLICT (code) DO UPDATE SET
			name = EXCLUDED.name, source_url = EXCLUDED.source_url,
			media_source_id = EXCLUDED.media_source_id, media_path = EXCLUDED.media_path,
			max_members = EXCLUDED.max_members, expires_at = EXCLUDED.expires_at,
			position = EXCLUDED.position, playing = EXCLUDED.playing,
			speed = EXCLUDED.speed, episode = EXCLUDED.episode,
			position_ts = EXCLUDED.position_ts, source_version = EXCLUDED.source_version,
			closed_at = COALESCE(rooms.closed_at, EXCLUDED.closed_at)`,
		room.Code,
		room.Name,
		room.OwnerID,
		room.SourceURL,
		nilIfEmpty(room.MediaSourceID),
		nilIfEmpty(room.MediaPath),
		room.MaxMembers,
		room.CreatedAt,
		room.ExpiresAt,
		room.Playback.Position,
		room.Playback.Playing,
		room.Playback.Speed,
		room.Playback.Episode,
		room.Playback.PositionTS,
		room.Playback.SourceVersion,
		closedAtValue(room.Closed),
	)
	if err != nil {
		return err
	}
	for _, member := range room.Members {
		if _, err = tx.Exec(
			ctx,
			`INSERT INTO room_members (room_code, user_id, display_name, joined_at)
			 VALUES ($1,$2,$3,$4) ON CONFLICT (room_code, user_id) DO NOTHING`,
			room.Code,
			member.UserID,
			member.DisplayName,
			member.JoinedAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) SaveMember(ctx context.Context, code string, member Member) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO room_members (room_code, user_id, display_name, joined_at)
		 VALUES ($1,$2,$3,$4) ON CONFLICT (room_code, user_id) DO NOTHING`,
		code,
		member.UserID,
		member.DisplayName,
		member.JoinedAt,
	)
	return err
}

func (r *PostgresRepository) UpdatePlayback(ctx context.Context, code string, playback Playback) error {
	_, err := r.pool.Exec(
		ctx,
		`UPDATE rooms SET position=$2, playing=$3, speed=$4, episode=$5,
		 position_ts=$6, source_version=$7 WHERE code=$1`,
		code,
		playback.Position,
		playback.Playing,
		playback.Speed,
		playback.Episode,
		playback.PositionTS,
		playback.SourceVersion,
	)
	return err
}

func (r *PostgresRepository) LoadRooms(ctx context.Context) ([]Room, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT code, name, owner_id, source_url,
		 COALESCE(media_source_id, ''), COALESCE(media_path, ''),
		 max_members, created_at, expires_at,
		 position, playing, speed, episode, position_ts, source_version,
		 closed_at IS NOT NULL FROM rooms`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rooms := make([]Room, 0)
	roomIndexes := make(map[string]int)
	for rows.Next() {
		var room Room
		if err := rows.Scan(
			&room.Code,
			&room.Name,
			&room.OwnerID,
			&room.SourceURL,
			&room.MediaSourceID,
			&room.MediaPath,
			&room.MaxMembers,
			&room.CreatedAt,
			&room.ExpiresAt,
			&room.Playback.Position,
			&room.Playback.Playing,
			&room.Playback.Speed,
			&room.Playback.Episode,
			&room.Playback.PositionTS,
			&room.Playback.SourceVersion,
			&room.Closed,
		); err != nil {
			return nil, err
		}
		room.Code = strings.TrimSpace(room.Code)
		roomIndexes[room.Code] = len(rooms)
		rooms = append(rooms, room)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	memberRows, err := r.pool.Query(
		ctx,
		`SELECT room_code, user_id, display_name, joined_at FROM room_members ORDER BY joined_at`,
	)
	if err != nil {
		return nil, err
	}
	defer memberRows.Close()
	for memberRows.Next() {
		var code string
		var member Member
		if err := memberRows.Scan(&code, &member.UserID, &member.DisplayName, &member.JoinedAt); err != nil {
			return nil, err
		}
		if index, ok := roomIndexes[strings.TrimSpace(code)]; ok {
			rooms[index].Members = append(rooms[index].Members, member)
		}
	}
	return rooms, memberRows.Err()
}

func (r *PostgresRepository) Close() {
	r.pool.Close()
}

func nilIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func closedAtValue(closed bool) any {
	if !closed {
		return nil
	}
	return time.Now()
}
