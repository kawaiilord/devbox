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

Check-ins use UTC dates and a `(user_id, check_date)` primary key. Every balance
change writes a ledger row with the resulting balance in the same transaction.
`SAMEFRAME_POINTS_PER_VIP_DAY=0` closes redemption; a positive value is the
number of points required per membership day. Redemption locks and debits the
points account before extending membership atomically.

## Room entitlement

Room creation reloads the current account from the database. Free rooms expire
after six hours. A member's room expires at their current membership expiry;
clients cannot extend this by modifying a token or request body.
