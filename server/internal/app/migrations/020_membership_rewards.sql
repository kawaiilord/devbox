-- Additive: previous server versions can continue using points and users.
CREATE TABLE IF NOT EXISTS membership_redemptions (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  request_id TEXT NOT NULL,
  points_cost INTEGER NOT NULL CHECK(points_cost > 0),
  duration_days INTEGER NOT NULL CHECK(duration_days > 0),
  balance_after BIGINT NOT NULL CHECK(balance_after >= 0),
  vip_expires_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY(user_id, request_id)
);
