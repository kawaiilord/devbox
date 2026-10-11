package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
)

type SecurityRepository interface {
	ReadAccountSecurity(context.Context, string) (AccountSecurity, error)
	WriteAccountSecurity(context.Context, string, int64, AccountSecurity, bool) (User, error)
	SaveAuthChallenge(context.Context, string, AuthChallenge) error
	ConsumeAuthChallenge(context.Context, string, string) (AuthChallenge, error)
	ExternalIdentity(context.Context, string, string) (AccountRecord, error)
	LinkExternalIdentity(context.Context, string, string, string) error
}
type NamedCredential struct {
	Name       string              `json:"name"`
	CreatedAt  int64               `json:"created_at"`
	Credential webauthn.Credential `json:"credential"`
}
type AccountSecurity struct {
	Version        int64             `json:"version"`
	TOTPSecret     string            `json:"totp_secret,omitempty"`
	LastStep       int64             `json:"last_step"`
	RecoveryHashes []string          `json:"recovery_hashes,omitempty"`
	Passkeys       []NamedCredential `json:"passkeys"`
}
type AuthChallenge struct {
	Purpose   string
	Data      json.RawMessage
	ExpiresAt time.Time
}

func cloneSecurity(v AccountSecurity) AccountSecurity {
	raw, _ := json.Marshal(v)
	var c AccountSecurity
	_ = json.Unmarshal(raw, &c)
	return c
}
func (r *MemoryRepository) ReadAccountSecurity(_ context.Context, id string) (AccountSecurity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.usersByID[id]; !ok {
		return AccountSecurity{}, ErrNotFound
	}
	return cloneSecurity(r.security[id]), nil
}
func (r *MemoryRepository) WriteAccountSecurity(_ context.Context, id string, sessionVersion int64, v AccountSecurity, revoke bool) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.usersByID[id]
	if !ok || !accountActive(a.User) || a.SessionVersion != sessionVersion {
		return User{}, ErrUnauthorized
	}
	if r.security[id].Version != v.Version {
		return User{}, ErrAdminConflict
	}
	v.Version++
	if r.security == nil {
		r.security = map[string]AccountSecurity{}
	}
	r.security[id] = cloneSecurity(v)
	if revoke {
		a.SessionVersion++
		r.usersByID[id] = a
		for k, t := range r.refresh {
			if t.userID == id {
				t.revoked = true
				r.refresh[k] = t
			}
		}
	}
	r.appendAuditLocked(AuditEvent{ActorID: id, Action: "account.security.update", TargetType: "user", TargetID: id})
	return a.User, nil
}
func (r *MemoryRepository) SaveAuthChallenge(_ context.Context, raw string, v AuthChallenge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.authChallenges == nil {
		r.authChallenges = map[string]AuthChallenge{}
	}
	for k, c := range r.authChallenges {
		if c.ExpiresAt.Before(time.Now()) {
			delete(r.authChallenges, k)
		}
	}
	key := hashRefreshToken(raw)
	if _, exists := r.authChallenges[key]; exists {
		return ErrAdminConflict
	}
	v.Data = append(json.RawMessage(nil), v.Data...)
	r.authChallenges[key] = v
	return nil
}
func (r *MemoryRepository) ConsumeAuthChallenge(_ context.Context, raw, purpose string) (AuthChallenge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := hashRefreshToken(raw)
	v, ok := r.authChallenges[key]
	if !ok || v.Purpose != purpose || !v.ExpiresAt.After(time.Now()) {
		return AuthChallenge{}, ErrInvalidActionToken
	}
	delete(r.authChallenges, key)
	v.Data = append(json.RawMessage(nil), v.Data...)
	return v, nil
}
func identityKey(provider, subject string) string {
	b, _ := json.Marshal([]string{provider, subject})
	return string(b)
}
func (r *MemoryRepository) ExternalIdentity(_ context.Context, provider, subject string) (AccountRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.usersByID[r.externalIdentities[identityKey(provider, subject)]]
	if !ok {
		return a, ErrNotFound
	}
	return a, nil
}
func (r *MemoryRepository) LinkExternalIdentity(_ context.Context, id, provider, subject string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.usersByID[id]
	if !ok || (!accountActive(a.User) && a.AccountStatus != "pending") {
		return ErrUnauthorized
	}
	key := identityKey(provider, subject)
	if _, ok := r.externalIdentities[key]; ok {
		return ErrAdminConflict
	}
	if r.externalIdentities == nil {
		r.externalIdentities = map[string]string{}
	}
	r.externalIdentities[key] = id
	return nil
}
func (r *PostgresRepository) ReadAccountSecurity(ctx context.Context, id string) (AccountSecurity, error) {
	var v AccountSecurity
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(s.version,0),COALESCE(s.data,'{}') FROM users u LEFT JOIN account_security s ON s.user_id=u.id WHERE u.id=$1`, id).Scan(&v.Version, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	version := v.Version
	err = json.Unmarshal(raw, &v)
	v.Version = version
	return v, err
}
func (r *PostgresRepository) WriteAccountSecurity(ctx context.Context, id string, sessionVersion int64, v AccountSecurity, revoke bool) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	a, err := scanAccount(tx.QueryRow(ctx, `SELECT `+adminAccountColumns+` FROM users WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return User{}, err
	}
	if !accountActive(a.User) || a.SessionVersion != sessionVersion {
		return User{}, ErrUnauthorized
	}
	var current int64
	err = tx.QueryRow(ctx, `SELECT version FROM account_security WHERE user_id=$1`, id).Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return User{}, err
	}
	if current != v.Version {
		return User{}, ErrAdminConflict
	}
	v.Version++
	raw, err := json.Marshal(v)
	if err != nil {
		return User{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO account_security(user_id,version,data) VALUES($1,$2,$3) ON CONFLICT(user_id) DO UPDATE SET version=EXCLUDED.version,data=EXCLUDED.data`, id, v.Version, raw); err != nil {
		return User{}, err
	}
	if revoke {
		a.SessionVersion++
		if _, err = tx.Exec(ctx, `UPDATE users SET session_version=session_version+1 WHERE id=$1`, id); err != nil {
			return User{}, err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id=$1`, id); err != nil {
			return User{}, err
		}
	}
	if _, err = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: id, Action: "account.security.update", TargetType: "user", TargetID: id}); err != nil {
		return User{}, err
	}
	return a.User, tx.Commit(ctx)
}
func (r *PostgresRepository) SaveAuthChallenge(ctx context.Context, raw string, v AuthChallenge) error {
	// The expiry index keeps short-lived ceremony records bounded without a scheduler.
	_, err := r.pool.Exec(ctx, `DELETE FROM auth_challenges WHERE expires_at<now()`)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO auth_challenges(token_hash,purpose,data,expires_at) VALUES($1,$2,$3,$4)`, hashRefreshToken(raw), v.Purpose, []byte(v.Data), v.ExpiresAt)
	return err
}
func (r *PostgresRepository) ConsumeAuthChallenge(ctx context.Context, raw, purpose string) (AuthChallenge, error) {
	var v AuthChallenge
	err := r.pool.QueryRow(ctx, `DELETE FROM auth_challenges WHERE token_hash=$1 AND purpose=$2 AND expires_at>now() RETURNING purpose,data,expires_at`, hashRefreshToken(raw), purpose).Scan(&v.Purpose, &v.Data, &v.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrInvalidActionToken
	}
	return v, err
}
func (r *PostgresRepository) ExternalIdentity(ctx context.Context, provider, subject string) (AccountRecord, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM external_identities WHERE provider=$1 AND subject=$2`, provider, subject).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountRecord{}, ErrNotFound
	}
	if err != nil {
		return AccountRecord{}, err
	}
	return r.UserByID(ctx, id)
}
func (r *PostgresRepository) LinkExternalIdentity(ctx context.Context, id, provider, subject string) error {
	tag, err := r.pool.Exec(ctx, `INSERT INTO external_identities(provider,subject,user_id) SELECT $1,$2,id FROM users WHERE id=$3 AND account_status IN ('active','pending') ON CONFLICT DO NOTHING`, provider, subject, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrAdminConflict
	}
	return nil
}
