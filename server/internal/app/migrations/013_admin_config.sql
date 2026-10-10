ALTER TABLE users ADD COLUMN IF NOT EXISTS admin_role TEXT NOT NULL DEFAULT '';
UPDATE users SET admin_role='super_admin' WHERE is_admin AND admin_role='';
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_admin_role_check;
ALTER TABLE users ADD CONSTRAINT users_admin_role_check CHECK(admin_role IN ('','super_admin','admin','operator','seller'));

CREATE TABLE IF NOT EXISTS runtime_config (
 key TEXT PRIMARY KEY, value JSONB NOT NULL, sensitive BOOLEAN NOT NULL DEFAULT false,
 updated_by TEXT REFERENCES users(id), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO runtime_config(key,value) VALUES
 ('maintenance', '{"enabled":false,"message":""}'::jsonb),
 ('features', '{}'::jsonb),
 ('branding', '{"global_announcement":"","startup_announcement_id":0}'::jsonb)
ON CONFLICT(key) DO NOTHING;

CREATE TABLE IF NOT EXISTS announcements (
 id BIGSERIAL PRIMARY KEY, title TEXT NOT NULL CHECK(char_length(title) BETWEEN 1 AND 160),
 body TEXT NOT NULL CHECK(char_length(body) BETWEEN 1 AND 5000), kind TEXT NOT NULL CHECK(kind IN ('list','startup','room')),
 active BOOLEAN NOT NULL DEFAULT true, starts_at TIMESTAMPTZ, ends_at TIMESTAMPTZ,
 created_by TEXT NOT NULL REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(ends_at IS NULL OR starts_at IS NULL OR ends_at>starts_at)
);
CREATE INDEX IF NOT EXISTS announcements_active_time ON announcements(active,starts_at,ends_at,id DESC);

CREATE TABLE IF NOT EXISTS room_bot_config (
 singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK(singleton), enabled BOOLEAN NOT NULL DEFAULT false,
 display_name TEXT NOT NULL DEFAULT 'SameFrame Bot', summon_policy TEXT NOT NULL DEFAULT 'admin'
  CHECK(summon_policy IN ('admin','vip','allowlist','all')),
 reply_policy TEXT NOT NULL DEFAULT 'mention' CHECK(reply_policy IN ('mention','all','off')),
 provider_base_url TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', credential_ciphertext TEXT NOT NULL DEFAULT '',
 updated_by TEXT REFERENCES users(id), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO room_bot_config(singleton) VALUES(true) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS room_bot_allowlist (
 user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 added_by TEXT NOT NULL REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE devices ADD COLUMN IF NOT EXISTS unbanned_at TIMESTAMPTZ;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS unbanned_by TEXT REFERENCES users(id);
ALTER TABLE devices ADD COLUMN IF NOT EXISTS banned_by TEXT REFERENCES users(id);
ALTER TABLE user_devices ADD COLUMN IF NOT EXISTS revoked_by_ban BOOLEAN NOT NULL DEFAULT false;
