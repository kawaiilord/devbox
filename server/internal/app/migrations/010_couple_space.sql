ALTER TABLE users ADD COLUMN IF NOT EXISTS vip_expires_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS couple_requests (
    id BIGSERIAL PRIMARY KEY,
    requester_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','rejected','cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at TIMESTAMPTZ,
    CHECK (requester_id <> recipient_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS couple_requests_pending_pair_idx
    ON couple_requests(requester_id,recipient_id) WHERE status='pending';

CREATE TABLE IF NOT EXISTS couples (
    id BIGSERIAL PRIMARY KEY,
    user_low TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_high TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','separated','ended')),
    bound_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    separated_at TIMESTAMPTZ,
    cooling_period_end TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (user_low < user_high)
);
CREATE UNIQUE INDEX IF NOT EXISTS couples_pair_idx ON couples(user_low,user_high);

CREATE TABLE IF NOT EXISTS couple_events (
    id BIGSERIAL PRIMARY KEY,
    couple_id BIGINT NOT NULL REFERENCES couples(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (event_type IN ('bound','separated','restored','moment')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS couple_moments (
    id BIGSERIAL PRIMARY KEY,
    couple_id BIGINT NOT NULL REFERENCES couples(id) ON DELETE CASCADE,
    author_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS couple_moments_page_idx ON couple_moments(couple_id,id DESC);
