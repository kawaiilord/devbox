CREATE TABLE IF NOT EXISTS reviews (
 id BIGSERIAL PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 target_type TEXT NOT NULL CHECK (target_type IN ('movie','tv','media')),
 target_id TEXT NOT NULL CHECK (char_length(target_id) BETWEEN 1 AND 256),
 title TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 256), rating INTEGER NOT NULL CHECK (rating BETWEEN 1 AND 10),
 content TEXT NOT NULL CHECK (char_length(content) BETWEEN 1 AND 5000), image_keys TEXT[] NOT NULL DEFAULT '{}',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(user_id,target_type,target_id)
);
CREATE INDEX IF NOT EXISTS reviews_target_page_idx ON reviews(target_type,target_id,updated_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS review_comments (
 id BIGSERIAL PRIMARY KEY, review_id BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS review_comments_page_idx ON review_comments(review_id,id);
