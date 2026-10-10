CREATE TABLE IF NOT EXISTS room_features (
    room_code CHAR(6) PRIMARY KEY REFERENCES rooms(code) ON DELETE CASCADE,
    version BIGINT NOT NULL DEFAULT 0,
    data JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS room_features_visibility_idx
    ON room_features ((data->>'visibility'));
