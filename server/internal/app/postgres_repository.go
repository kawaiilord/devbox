package app

import (
	"context"
	_ "embed"
	"errors"
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
	for _, migration := range []string{foundationMigration, accountSecurityMigration, mediaSourcesMigration} {
		if _, err := r.pool.Exec(ctx, migration); err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepository) CreateUser(ctx context.Context, account AccountRecord) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO users (id, email, display_name, password_hash, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		account.ID,
		strings.ToLower(account.Email),
		account.DisplayName,
		account.PasswordHash,
		account.CreatedAt,
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
		 email_verified_at IS NOT NULL, session_version
		 FROM users WHERE email = lower($1)`,
		email,
	))
}

func (r *PostgresRepository) UserByID(ctx context.Context, id string) (AccountRecord, error) {
	return scanAccount(r.pool.QueryRow(
		ctx,
		`SELECT id, email, display_name, password_hash, created_at,
		 email_verified_at IS NOT NULL, session_version
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
		 email_verified_at IS NOT NULL, session_version FROM users WHERE id = $1`,
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
		 email_verified_at IS NOT NULL, session_version FROM users WHERE id = $1`,
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
			position, playing, speed, episode, position_ts, source_version
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 ON CONFLICT (code) DO UPDATE SET
			name = EXCLUDED.name, source_url = EXCLUDED.source_url,
			media_source_id = EXCLUDED.media_source_id, media_path = EXCLUDED.media_path,
			max_members = EXCLUDED.max_members, expires_at = EXCLUDED.expires_at,
			position = EXCLUDED.position, playing = EXCLUDED.playing,
			speed = EXCLUDED.speed, episode = EXCLUDED.episode,
			position_ts = EXCLUDED.position_ts, source_version = EXCLUDED.source_version`,
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
		 position, playing, speed, episode, position_ts, source_version FROM rooms`,
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
