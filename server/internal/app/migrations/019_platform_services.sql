ALTER TABLE users DROP CONSTRAINT IF EXISTS users_account_status_check;
ALTER TABLE users ADD CONSTRAINT users_account_status_check CHECK(account_status IN ('active','banned','deleted','pending','rejected'));
CREATE TABLE IF NOT EXISTS approval_requests (
 id TEXT PRIMARY KEY,
 kind TEXT NOT NULL CHECK(kind IN ('registration','room_creation','room_join')),
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 room_code CHAR(6) REFERENCES rooms(code) ON DELETE CASCADE,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected','cancelled')),
 version BIGINT NOT NULL DEFAULT 1,
 reason TEXT NOT NULL DEFAULT '',
 created_at BIGINT NOT NULL,
 reviewed_at BIGINT NOT NULL DEFAULT 0,
 reviewed_by TEXT REFERENCES users(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS approval_one_pending ON approval_requests(kind,user_id,COALESCE(room_code,'')) WHERE status='pending';
CREATE INDEX IF NOT EXISTS approval_queue_page ON approval_requests(status,kind,created_at DESC,id);
CREATE TABLE IF NOT EXISTS account_security (
 user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 version BIGINT NOT NULL DEFAULT 1,
 data JSONB NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS auth_challenges (
 token_hash TEXT PRIMARY KEY,
 purpose TEXT NOT NULL,
 data JSONB NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS auth_challenges_expiry ON auth_challenges(expires_at);
CREATE TABLE IF NOT EXISTS external_identities (
 provider TEXT NOT NULL,
 subject TEXT NOT NULL,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 PRIMARY KEY(provider,subject)
);

-- Preserve a newer constraint on restart and when rolling back application code.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media_sources'::regclass
   AND conname='media_sources_source_type_check' AND pg_get_constraintdef(oid) NOT LIKE '%remote-alist%') THEN
  ALTER TABLE media_sources DROP CONSTRAINT media_sources_source_type_check;
  ALTER TABLE media_sources ADD CONSTRAINT media_sources_source_type_check CHECK(source_type IN ('webdav','emby','quark','synology','qnap','fnos','nextcloud','seafile','truenas','bilibili','youtube','douyin','tiktok','twitch','huya','douyu','acfun','cctv','remote-alist','remote-emby','remote-bilibili','rtmp'));
 END IF;
END $$;

CREATE TABLE IF NOT EXISTS room_streams(room_code CHAR(6) PRIMARY KEY REFERENCES rooms(code) ON DELETE CASCADE,version BIGINT NOT NULL,data JSONB NOT NULL);
