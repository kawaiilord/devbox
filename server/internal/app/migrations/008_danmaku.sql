CREATE TABLE IF NOT EXISTS danmaku_messages (
    id BIGSERIAL PRIMARY KEY,
    media_fingerprint CHAR(64) NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 32),
    body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 100),
    position_seconds DOUBLE PRECISION NOT NULL
        CHECK (position_seconds >= 0 AND position_seconds <= 2592000),
    color INTEGER NOT NULL DEFAULT 16777215 CHECK (color BETWEEN 0 AND 16777215),
    mode TEXT NOT NULL DEFAULT 'scroll' CHECK (mode IN ('scroll', 'top', 'bottom')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS danmaku_fingerprint_position_idx
    ON danmaku_messages(media_fingerprint, position_seconds, id);
