CREATE TABLE IF NOT EXISTS favorites (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_source_id TEXT NOT NULL REFERENCES media_sources(id) ON DELETE CASCADE,
    media_path TEXT NOT NULL CHECK (char_length(media_path) BETWEEN 1 AND 4096),
    title TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 256),
    content_type TEXT NOT NULL DEFAULT '' CHECK (char_length(content_type) <= 128),
    size BIGINT NOT NULL DEFAULT 0 CHECK (size >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, media_source_id, media_path)
);

CREATE INDEX IF NOT EXISTS favorites_user_updated_idx
    ON favorites(user_id, updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS watch_records (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_key CHAR(64) NOT NULL,
    media_source_id TEXT REFERENCES media_sources(id) ON DELETE SET NULL,
    media_path TEXT NOT NULL DEFAULT '' CHECK (char_length(media_path) <= 4096),
    title TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 256),
    position_seconds DOUBLE PRECISION NOT NULL DEFAULT 0
        CHECK (position_seconds >= 0 AND position_seconds <= 2592000),
    duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0
        CHECK (duration_seconds >= 0 AND duration_seconds <= 2592000),
    episode INTEGER NOT NULL DEFAULT 0 CHECK (episode >= 0),
    completed BOOLEAN NOT NULL DEFAULT false,
    companion_count INTEGER NOT NULL DEFAULT 0 CHECK (companion_count BETWEEN 0 AND 100),
    room_code CHAR(6),
    watched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, media_key)
);

CREATE INDEX IF NOT EXISTS watch_records_user_watched_idx
    ON watch_records(user_id, watched_at DESC, id DESC);
