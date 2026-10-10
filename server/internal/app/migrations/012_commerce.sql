CREATE TABLE IF NOT EXISTS vip_plans (
 id TEXT PRIMARY KEY, title TEXT NOT NULL, price_minor BIGINT NOT NULL CHECK(price_minor>=0),
 original_price_minor BIGINT NOT NULL DEFAULT 0 CHECK(original_price_minor>=0),
 duration_days INTEGER NOT NULL CHECK(duration_days>=0), lifetime BOOLEAN NOT NULL DEFAULT false,
 popular BOOLEAN NOT NULL DEFAULT false, enabled BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(lifetime OR duration_days>0)
);
INSERT INTO vip_plans(id,title,price_minor,original_price_minor,duration_days,lifetime,popular) VALUES
 ('monthly','月度会员',800,0,30,false,false),
 ('annual','年度会员',5800,9800,365,false,true),
 ('lifetime','终身会员',4900,13600,0,true,false)
ON CONFLICT(id) DO NOTHING;

CREATE TABLE IF NOT EXISTS payment_orders (
 order_no TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), plan_id TEXT NOT NULL REFERENCES vip_plans(id),
 plan_title TEXT NOT NULL, amount_minor BIGINT NOT NULL CHECK(amount_minor>=0), currency TEXT NOT NULL DEFAULT 'CNY',
 status TEXT NOT NULL CHECK(status IN ('pending','paid','activated','expired','cancelled')),
 pay_channel TEXT NOT NULL DEFAULT '', pay_trade_no TEXT, checkout_url TEXT NOT NULL DEFAULT '',
 qr_expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 paid_at TIMESTAMPTZ, activated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS payment_orders_trade_unique ON payment_orders(pay_channel,pay_trade_no) WHERE pay_trade_no IS NOT NULL;
CREATE INDEX IF NOT EXISTS payment_orders_user_created ON payment_orders(user_id,created_at DESC);

CREATE TABLE IF NOT EXISTS activation_codes (
 id BIGSERIAL PRIMARY KEY, code_hash TEXT NOT NULL UNIQUE, batch_id TEXT NOT NULL,
 plan_id TEXT REFERENCES vip_plans(id), duration_days INTEGER NOT NULL DEFAULT 0 CHECK(duration_days>=0),
 used_by TEXT REFERENCES users(id), used_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(plan_id IS NOT NULL OR duration_days>0), CHECK((used_by IS NULL)=(used_at IS NULL))
);
CREATE INDEX IF NOT EXISTS activation_codes_batch ON activation_codes(batch_id);

CREATE TABLE IF NOT EXISTS points_accounts (
 user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 balance BIGINT NOT NULL DEFAULT 0 CHECK(balance>=0), total_earned BIGINT NOT NULL DEFAULT 0 CHECK(total_earned>=0),
 total_spent BIGINT NOT NULL DEFAULT 0 CHECK(total_spent>=0), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS daily_check_ins (
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, check_date DATE NOT NULL, points INTEGER NOT NULL CHECK(points>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(user_id,check_date)
);
CREATE INDEX IF NOT EXISTS daily_check_ins_date ON daily_check_ins(check_date);
CREATE TABLE IF NOT EXISTS points_transactions (
 id BIGSERIAL PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 change BIGINT NOT NULL CHECK(change<>0), balance_after BIGINT NOT NULL CHECK(balance_after>=0),
 type TEXT NOT NULL CHECK(type IN ('check_in','redeem_vip','invite','admin')),
 ref_id TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS points_transactions_user_page ON points_transactions(user_id,id DESC);
