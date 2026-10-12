-- Additive. Keeping used_at prevents a deleted account from freeing its code.
CREATE TABLE IF NOT EXISTS registration_invites (
    id TEXT PRIMARY KEY,
    code_hash TEXT NOT NULL UNIQUE,
    hint TEXT NOT NULL,
    batch_id TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL DEFAULT 0,
    used_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    used_at BIGINT NOT NULL DEFAULT 0,
    revoked_at BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS registration_invites_batch ON registration_invites(batch_id);
CREATE INDEX IF NOT EXISTS registration_invites_created ON registration_invites(created_at DESC,id DESC);
