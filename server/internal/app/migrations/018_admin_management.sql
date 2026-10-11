ALTER TABLE users ADD COLUMN IF NOT EXISTS account_status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE users ADD COLUMN IF NOT EXISTS status_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS admin_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_account_status_check;
ALTER TABLE users ADD CONSTRAINT users_account_status_check CHECK (account_status IN ('active','banned','deleted'));
CREATE INDEX IF NOT EXISTS users_admin_list_idx ON users(account_status,created_at DESC,id);

ALTER TABLE media_sources ADD COLUMN IF NOT EXISTS disabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE media_sources ADD COLUMN IF NOT EXISTS admin_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS admin_epoch BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS admin_documents (
    key TEXT PRIMARY KEY,
    version BIGINT NOT NULL DEFAULT 1,
    data JSONB NOT NULL,
    updated_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
