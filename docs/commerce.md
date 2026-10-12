# Membership and commerce

SameFrame treats membership as server-authoritative state. Clients render plans
returned by `GET /api/v1/vip`; they never submit an amount. Order creation looks
up the enabled plan and snapshots its title and price into an immutable order.

## Payment provider

Set the following variables to enable payment orders:

```text
SAMEFRAME_PAYMENT_PROVIDER=hmac
SAMEFRAME_PAYMENT_CHECKOUT_URL=https://pay.example.com/checkout
SAMEFRAME_PAYMENT_CALLBACK_SECRET=<at least 32 random characters>
```

The built-in provider appends `order_no`, `amount_minor`, and `currency` to the
checkout URL. A payment adapter or gateway posts JSON to
`POST /api/v1/payments/callback`:

```json
{
  "order_no": "SF...",
  "trade_no": "provider-unique-id",
  "amount_minor": 800,
  "timestamp": 1791561600,
  "signature": "lowercase-hex-hmac"
}
```

The signature is HMAC-SHA256 over this exact UTF-8 payload, with newline
separators and no trailing newline:

```text
order_no\ntrade_no\namount_minor\ntimestamp
```

Callbacks outside a five-minute window are rejected. The transaction locks the
order and user, verifies the server-side amount, enforces provider trade-number
uniqueness, extends membership, and activates the order atomically. Repeating
the same callback returns success with `activated: false`; changing any payment
identity on a completed order is rejected.

The generic HMAC adapter is intentionally not tied to any payment vendor. A
production gateway should translate its native, independently verified webhook
into this callback or implement the `PaymentProvider` interface directly.

## Activation codes

Verified administrators create batches through
`POST /api/v1/admin/activation-codes`. Plain codes are returned once; only
SHA-256 hashes are stored. Redemption and membership extension occur in one
database transaction, so a code cannot be used twice under concurrency. Export
the one-time response directly into an access-controlled secrets workflow.

Verified administrators can extend or clear membership with
`PUT /api/v1/admin/users/{id}/vip` and reconcile an order with
`POST /api/v1/admin/orders/{order_no}/activate`. Both actions are written to the
tamper-evident administrator audit chain. Manual activation is idempotent for an
already activated order.

## Check-ins and points

```text
SAMEFRAME_POINTS_PER_CHECK_IN=1
SAMEFRAME_POINTS_PER_VIP_DAY=0
```

Check-ins use UTC dates (08:00 in Asia/Shanghai) and a `(user_id, check_date)`
primary key. Repeating today's check-in returns the current status without
awarding points twice. Every balance change writes a ledger row with the resulting
balance in the same transaction.

The current client uses the fixed monthly reward returned as `monthly_reward`
by `GET /api/v1/check-ins/status`: **30 points for 30 days**. To redeem, send:

```json
{"request_id":"a-unique-uuid-for-this-confirmed-redemption"}
```

to `POST /api/v1/points/redeem-month`. Keep the same request identifier when
retrying an uncertain response. The client persists pending request IDs across
page reloads and offers a result-recovery action even when the balance has already
been debited. Definite business rejections clear the pending request. The server fixes the cost and duration, locks the
user before the points account, and atomically writes the debit, membership expiry,
ledger and receipt. Receipts are scoped to the user. Insufficient points and
lifetime membership do not debit the balance. An active membership extends from its
expiry; an expired membership starts from successful redemption time.

`SAMEFRAME_POINTS_PER_VIP_DAY` applies only to the retained legacy
`/points/redeem-vip` API. Zero leaves that legacy API disabled; it does not disable
or reprice the fixed monthly reward. Existing clients remain compatible.

`go run ./cmd/rewards-fixture --credentials /private/path/rewards.json` runs a
loopback-only, in-memory browser fixture with 29 historical one-point check-ins.
It has no connection to the production database. One real UI check-in reaches 30
points and allows the monthly redemption workflow to be tested without modifying
real users. Remove its generated credentials after testing.

## Room entitlement

Room creation reloads the current account from the database. Free rooms expire
after six hours. A member's room expires at their current membership expiry;
clients cannot extend this by modifying a token or request body.
