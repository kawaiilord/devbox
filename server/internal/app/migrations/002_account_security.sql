ALTER TABLE users
    ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS session_version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE refresh_tokens
    ADD COLUMN IF NOT EXISTS device_hash CHAR(64);

CREATE INDEX IF NOT EXISTS refresh_tokens_device_idx
    ON refresh_tokens(user_id, device_hash) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS devices (
    device_hash CHAR(64) PRIMARY KEY,
    label TEXT NOT NULL,
    platform TEXT NOT NULL,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
    banned_at TIMESTAMPTZ,
    ban_reason TEXT
);

CREATE TABLE IF NOT EXISTS user_devices (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_hash CHAR(64) NOT NULL REFERENCES devices(device_hash) ON DELETE CASCADE,
    linked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    PRIMARY KEY (user_id, device_hash)
);

CREATE TABLE IF NOT EXISTS auth_action_tokens (
    token_hash CHAR(64) PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL CHECK (purpose IN ('verify_email', 'reset_password')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS auth_action_tokens_lookup_idx
    ON auth_action_tokens(purpose, expires_at) WHERE used_at IS NULL;
