ALTER TABLE payment_orders DROP CONSTRAINT IF EXISTS payment_orders_user_id_fkey;
ALTER TABLE payment_orders ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS deleted_user_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_orders ADD CONSTRAINT payment_orders_user_id_fkey FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE admin_audit_logs DROP CONSTRAINT IF EXISTS admin_audit_logs_actor_id_fkey;
ALTER TABLE reports DROP CONSTRAINT IF EXISTS reports_reviewed_by_fkey;
ALTER TABLE reports ADD CONSTRAINT reports_reviewed_by_fkey FOREIGN KEY(reviewed_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE runtime_config DROP CONSTRAINT IF EXISTS runtime_config_updated_by_fkey;
ALTER TABLE runtime_config ADD CONSTRAINT runtime_config_updated_by_fkey FOREIGN KEY(updated_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE announcements DROP CONSTRAINT IF EXISTS announcements_created_by_fkey;
ALTER TABLE announcements ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE announcements ADD CONSTRAINT announcements_created_by_fkey FOREIGN KEY(created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE room_bot_config DROP CONSTRAINT IF EXISTS room_bot_config_updated_by_fkey;
ALTER TABLE room_bot_config ADD CONSTRAINT room_bot_config_updated_by_fkey FOREIGN KEY(updated_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_banned_by_fkey;
ALTER TABLE devices ADD CONSTRAINT devices_banned_by_fkey FOREIGN KEY(banned_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_unbanned_by_fkey;
ALTER TABLE devices ADD CONSTRAINT devices_unbanned_by_fkey FOREIGN KEY(unbanned_by) REFERENCES users(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS account_deletion_requests (
 id BIGSERIAL PRIMARY KEY, user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
 user_ref TEXT NOT NULL, email_hash TEXT NOT NULL, reason TEXT NOT NULL CHECK(char_length(reason) BETWEEN 1 AND 1000),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected','cancelled')),
 requested_at TIMESTAMPTZ NOT NULL DEFAULT now(), reviewed_by TEXT REFERENCES users(id),
 reviewed_at TIMESTAMPTZ, resolution TEXT NOT NULL DEFAULT '', executed_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS account_deletion_pending_user ON account_deletion_requests(user_id) WHERE status='pending' AND user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS account_deletion_status_page ON account_deletion_requests(status,id DESC);

CREATE TABLE IF NOT EXISTS copyright_complaints (
 id BIGSERIAL PRIMARY KEY, claimant_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
 claimant_name TEXT NOT NULL CHECK(char_length(claimant_name) BETWEEN 1 AND 160),
 claimant_email TEXT NOT NULL CHECK(char_length(claimant_email) BETWEEN 3 AND 320),
 rights_basis TEXT NOT NULL CHECK(char_length(rights_basis) BETWEEN 10 AND 3000),
 infringement_url TEXT NOT NULL CHECK(char_length(infringement_url) BETWEEN 8 AND 2048),
 room_code CHAR(6), evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
 statement_accurate BOOLEAN NOT NULL, signature_name TEXT NOT NULL CHECK(char_length(signature_name) BETWEEN 1 AND 160),
 status TEXT NOT NULL DEFAULT 'submitted' CHECK(status IN ('submitted','triaged','actioned','rejected','withdrawn')),
 submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(), due_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '24 hours',
 reviewed_by TEXT REFERENCES users(id), reviewed_at TIMESTAMPTZ, resolution TEXT NOT NULL DEFAULT '',
 CHECK(statement_accurate)
);
CREATE INDEX IF NOT EXISTS copyright_complaints_status_due ON copyright_complaints(status,due_at,id);
