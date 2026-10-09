# Account security contract

## Device headers

Every client request sends:

```http
X-Device-ID: <installation UUID>
X-Device-Name: <user-facing label>
X-Device-Platform: windows|linux|web|...
```

The raw identifier is kept only in client preferences and request memory. The
server stores its SHA-256 digest. Access and refresh tokens are bound to that
digest; an access token presented by a different or revoked installation is
rejected.

## Verification and recovery

| Endpoint | Authentication | Result |
|---|---|---|
| `POST /api/v1/auth/email/verify-request` | access token | Replaces and mails a 24-hour token |
| `POST /api/v1/auth/email/verify` | public + token | Marks email verified; token is single-use |
| `POST /api/v1/auth/password/request` | public | Always returns the same accepted response |
| `POST /api/v1/auth/password/reset` | public + token | Replaces password and invalidates all sessions |
| `GET /api/v1/devices` | access token | Lists active installations |
| `DELETE /api/v1/devices/{digest}` | access token | Revokes another installation and its refresh tokens |

Password reset increments the account session version. Existing access tokens
fail immediately when their embedded version no longer matches PostgreSQL.

## Mail webhook

When configured, the server sends an authenticated HTTPS `POST` to
`SAMEFRAME_MAIL_WEBHOOK_URL` with:

```json
{
  "type": "verify_email",
  "to": "person@example.com",
  "token": "opaque-random-value",
  "expires_at": "2026-10-10T06:00:00Z"
}
```

`Authorization: Bearer <SAMEFRAME_MAIL_WEBHOOK_SECRET>` is included. The mail
adapter must turn this into a link whose origin is configured by the adapter,
not copied from an incoming Host header. Tokens must never be logged.

## Redis fixed-window limits

| Flow | Limit |
|---|---:|
| Registration | 5/device and 20/source per hour |
| Login | 10/source and 20/account per 15 minutes |
| Refresh | 30/device and 100/source per minute |
| Verification mail | 5 per hour |
| Verification attempts | 15 per 15 minutes |
| Password-reset mail | 5/account and 20/source per hour |
| Password-reset attempts | 10 per 30 minutes |

Limits are atomic across service nodes. A rejected response uses HTTP 429 and
includes `Retry-After`; successful checks include `X-RateLimit-Remaining`.
