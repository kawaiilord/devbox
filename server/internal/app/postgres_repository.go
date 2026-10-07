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
	_, err := r.pool.Exec(ctx, foundationMigration)
	return err
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
		`SELECT id, email, display_name, password_hash, created_at
		 FROM users WHERE email = lower($1)`,
		email,
	))
}

func (r *PostgresRepository) UserByID(ctx context.Context, id string) (AccountRecord, error) {
	return scanAccount(r.pool.QueryRow(
		ctx,
		`SELECT id, email, display_name, password_hash, created_at
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
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountRecord{}, ErrInvalidCredentials
	}
	return account, err
}

func (r *PostgresRepository) StoreRefreshToken(
	ctx context.Context,
	userID, hash string,
	expiresAt time.Time,
) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO refresh_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		hash,
		userID,
		expiresAt,
	)
	return err
}

func (r *PostgresRepository) RotateRefreshToken(
	ctx context.Context,
	oldHash, newHash string,
	expiresAt time.Time,
) (AccountRecord, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return AccountRecord{}, err
	}
	defer tx.Rollback(ctx)
	var userID string
	err = tx.QueryRow(
		ctx,
		`SELECT user_id FROM refresh_tokens
		 WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
		 FOR UPDATE`,
		oldHash,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountRecord{}, ErrInvalidRefresh
	}
	if err != nil {
		return AccountRecord{}, err
	}
	if _, err = tx.Exec(
		ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1`,
		oldHash,
	); err != nil {
		return AccountRecord{}, err
	}
	if _, err = tx.Exec(
		ctx,
		`INSERT INTO refresh_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		newHash,
		userID,
		expiresAt,
	); err != nil {
		return AccountRecord{}, err
	}
	account, err := scanAccount(tx.QueryRow(
		ctx,
		`SELECT id, email, display_name, password_hash, created_at FROM users WHERE id = $1`,
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

func (r *PostgresRepository) RevokeRefreshToken(ctx context.Context, hash string) error {
	_, err := r.pool.Exec(
		ctx,
		`UPDATE refresh_tokens SET revoked_at = COALESCE(revoked_at, now()) WHERE token_hash = $1`,
		hash,
	)
	return err
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
			code, name, owner_id, source_url, max_members, created_at, expires_at,
			position, playing, speed, episode, position_ts, source_version
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 ON CONFLICT (code) DO UPDATE SET
			name = EXCLUDED.name, source_url = EXCLUDED.source_url,
			max_members = EXCLUDED.max_members, expires_at = EXCLUDED.expires_at,
			position = EXCLUDED.position, playing = EXCLUDED.playing,
			speed = EXCLUDED.speed, episode = EXCLUDED.episode,
			position_ts = EXCLUDED.position_ts, source_version = EXCLUDED.source_version`,
		room.Code,
		room.Name,
		room.OwnerID,
		room.SourceURL,
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
		`SELECT code, name, owner_id, source_url, max_members, created_at, expires_at,
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
