# Administration and runtime configuration

The administration console is an independent static web application served at
`/admin/`. It is not compiled into the Flutter client. The server embeds its
files for a single deployable binary and serves them with a restrictive CSP,
`no-store`, no framing, and no external script dependencies.

Administrators sign in with the normal account endpoint. Every admin API
reloads the current database account and checks its role and verified-email
state; stale JWT role claims cannot grant access.

## Roles

- `super_admin`: all permissions, including runtime configuration, roles, plans,
  device controls, and VIP adjustments.
- `admin`: moderation, announcements/broadcasts, Bot management, device
  ban/unban, order reconciliation, VIP adjustments, dashboard, and audit.
- `operator`: dashboard, user lookup, announcements, and broadcasts.
- `seller`: order visibility and one-time activation-code generation.

Only a super administrator can assign roles. Self-demotion through the role API
is rejected to reduce accidental lockout. Bootstrap super administrators with
`SAMEFRAME_ADMIN_EMAILS`; the startup provisioner writes the role to the
database.

## Configuration and maintenance

Runtime configuration is stored in PostgreSQL as validated JSON documents.
Changing it requires both `config.write` and the explicit header
`X-Confirm-Dangerous: update-runtime-config`. Every change is appended to the
tamper-evident audit chain.

Maintenance mode rejects ordinary API and WebSocket work with HTTP 503 and a
`Retry-After` header. Health, public config and announcements, authentication,
payment callbacks, and administrative recovery endpoints remain available.

Feature overrides are merged onto capability-derived defaults. This allows an
operator to turn off a capability but does not create credentials or external
services that are absent.

## Announcements

Announcements support list, startup, and room kinds plus optional start/end
times. Active list/startup announcements are publicly readable. A room
announcement is also sent to all currently connected room sockets as an
`announcement` envelope; the Flutter room renders a dismissible banner.
Ephemeral all-room broadcasts are audited but not added to the announcement
list.

## Devices

A ban revokes active refresh tokens and linked device sessions. The database
records which links were revoked specifically by that ban. Unban restores only
those links; a user-initiated device revocation remains revoked. Existing
refresh tokens stay invalid, so the user must authenticate again.

## Bot credentials

Bot provider credentials require `SAMEFRAME_VAULT_KEY` and use the same
AES-GCM credential vault as media sources, with distinct associated data. The
API returns only `has_credential`; plaintext and ciphertext are never returned.
Leaving the credential field empty preserves the existing secret.

The current stage supplies Bot identity, policy, provider, model, allowlist
schema, and encrypted credentials. Outbound AI requests should additionally be
protected by provider-specific egress allowlists, response limits, and abuse
controls before enabling automated replies in production.
