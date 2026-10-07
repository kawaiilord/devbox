CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE CHECK (email = lower(email)),
    display_name TEXT NOT NULL CHECK (char_length(display_name) BETWEEN 2 AND 32),
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS refresh_tokens_user_id_idx ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS refresh_tokens_expiry_idx ON refresh_tokens(expires_at) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS rooms (
    code CHAR(6) PRIMARY KEY,
    name TEXT NOT NULL,
    owner_id TEXT NOT NULL REFERENCES users(id),
    source_url TEXT NOT NULL,
    max_members INTEGER NOT NULL CHECK (max_members BETWEEN 2 AND 100),
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL,
    position DOUBLE PRECISION NOT NULL DEFAULT 0,
    playing BOOLEAN NOT NULL DEFAULT false,
    speed DOUBLE PRECISION NOT NULL DEFAULT 1,
    episode INTEGER NOT NULL DEFAULT 0,
    position_ts BIGINT NOT NULL,
    source_version BIGINT NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS rooms_owner_id_idx ON rooms(owner_id);
CREATE INDEX IF NOT EXISTS rooms_expires_at_idx ON rooms(expires_at);

CREATE TABLE IF NOT EXISTS room_members (
    room_code CHAR(6) NOT NULL REFERENCES rooms(code) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    joined_at BIGINT NOT NULL,
    PRIMARY KEY (room_code, user_id)
);
