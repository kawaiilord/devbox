ALTER TABLE users
    ADD COLUMN IF NOT EXISTS signature TEXT NOT NULL DEFAULT ''
    CHECK (char_length(signature) <= 160);

ALTER TABLE user_privacy
    ADD COLUMN IF NOT EXISTS allow_private_chat BOOLEAN NOT NULL DEFAULT true;

CREATE TABLE IF NOT EXISTS user_follows (
    follower_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followed_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followed_id),
    CHECK (follower_id <> followed_id)
);

CREATE INDEX IF NOT EXISTS user_follows_followed_idx ON user_follows(followed_id);

CREATE TABLE IF NOT EXISTS direct_conversations (
    id BIGSERIAL PRIMARY KEY,
    user_low TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_high TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_low, user_high),
    CHECK (user_low < user_high)
);

CREATE TABLE IF NOT EXISTS direct_messages (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES direct_conversations(id) ON DELETE CASCADE,
    sender_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS direct_messages_page_idx
    ON direct_messages(conversation_id, id DESC);

CREATE TABLE IF NOT EXISTS conversation_reads (
    conversation_id BIGINT NOT NULL REFERENCES direct_conversations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_read_message_id BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, user_id)
);
