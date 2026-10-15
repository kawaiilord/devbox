CREATE TABLE IF NOT EXISTS registration_records (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    registered_at BIGINT NOT NULL,
    source_ip TEXT NOT NULL,
    network_key TEXT NOT NULL,
    device_hash TEXT NOT NULL,
    browser_hash TEXT NOT NULL,
    environment_hash TEXT NOT NULL,
    method TEXT NOT NULL,
    data JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS registration_records_ip ON registration_records(source_ip);
CREATE INDEX IF NOT EXISTS registration_records_network ON registration_records(network_key);
CREATE INDEX IF NOT EXISTS registration_records_device ON registration_records(device_hash);
CREATE INDEX IF NOT EXISTS registration_records_browser ON registration_records(browser_hash) WHERE browser_hash <> '';
CREATE INDEX IF NOT EXISTS registration_records_environment ON registration_records(environment_hash) WHERE environment_hash <> '';

-- Anonymous counters intentionally survive account deletion. Only a successful
-- registration commits quota consumption; old application versions can ignore
-- these additive tables safely.
CREATE TABLE IF NOT EXISTS registration_quotas (
    key_hash TEXT PRIMARY KEY,
    window_start BIGINT NOT NULL,
    registrations INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS registration_quotas_window ON registration_quotas(window_start);
