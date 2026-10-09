CREATE TABLE IF NOT EXISTS media_sources (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL CHECK (source_type IN ('webdav')),
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
    base_url TEXT NOT NULL,
    credentials_ciphertext TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS media_sources_user_id_idx ON media_sources(user_id);

ALTER TABLE rooms
    ADD COLUMN IF NOT EXISTS media_source_id TEXT REFERENCES media_sources(id) ON DELETE SET NULL;

ALTER TABLE rooms
    ADD COLUMN IF NOT EXISTS media_path TEXT;
