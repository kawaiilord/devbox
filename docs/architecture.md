# SameFrame architecture

## Clean-room boundary

This repository implements the ideas in the supplied blueprint with original
code and a new name and interface. It does not call the analyzed service, copy
its assets, or accept third-party cookies and media-library credentials.

## Milestone 1 topology

```text
Flutter client ── HTTPS ──► Go API nodes ──► PostgreSQL
       │                         │             durable business state
       ├── WebSocket ticket ─────┤
       ├── WSS room channel ◄────┤──► Redis
       └── direct HTTP media     │    hot playback / PubSub / presence
```

WebDAV credentials and Emby session tokens are encrypted PostgreSQL records.
API nodes decrypt them only while making an outbound request through an
SSRF-restricted transport. Players consume a renewable SameFrame media ticket,
optionally through the desktop loopback Range cache; they never receive provider
credentials.

Chat is written to PostgreSQL before a canonical message envelope is published
through Redis to every WebSocket node. Each node checks both directions of the
block relationship before delivering a chat event, and history applies the same
filter in PostgreSQL. External subtitles reuse the media-ticket proxy but are
restricted to the room video’s directory.

Durable entities are restored from PostgreSQL at startup. Redis holds the hot
playback hashes, globally increasing room sequences, one-time socket tickets,
online-presence sorted sets, and the cross-node Pub/Sub channel. If Redis loses
ephemeral data, room state is reconstructed from PostgreSQL on service startup.

## Authentication

- Passwords are normalized only at the email boundary and hashed with Argon2id
  using per-password random salts.
- HMAC-SHA256 access tokens expire after 15 minutes and validate issuer,
  audience, validity time, and the exact signing algorithm.
- Opaque refresh tokens live for 30 days. Only SHA-256 token digests are stored;
  every refresh transaction revokes the old token and inserts a new one.
- Access-token authentication checks that the account still exists in the
  authoritative database.
- Login performs a dummy Argon2id verification for unknown emails to reduce
  obvious account-enumeration timing differences.

## Account and device security

- Each installation creates a random UUID in local preferences. It is not a
  secret, but every API request carries it alongside a user-facing device label
  and platform. The server stores only its SHA-256 digest.
- Access tokens contain the device digest. Requests must present the matching
  raw device ID and an active user-device link; revocation therefore blocks an
  access token immediately instead of waiting 15 minutes for expiry.
- Refresh tokens are also bound to a device digest. Revoking a device revokes
  its refresh tokens, and a revoked device cannot silently relink by logging in.
- Verification and recovery identifiers are 256-bit random values. Only their
  digests are stored; replacing a token invalidates its predecessor and
  consuming it is a transactionally enforced one-time operation.
- Password-reset requests always return the same response. Delivery is queued
  outside the request path to reduce account-enumeration timing signals.
- Redis Lua scripts atomically increment fixed-window counters and set their
  expiration. Limits combine the direct peer address, device ID, and normalized
  account identifier as appropriate. Deployments behind a proxy should enforce
  additional edge limits and configure the proxy so clients cannot spoof source
  addresses.
- Raw verification/reset tokens are sent only to an authenticated HTTPS mail
  webhook. They are never written to application logs or returned by APIs.

Local Compose sets `SAMEFRAME_REQUIRE_VERIFIED_EMAIL=false` so the demo remains
usable without a mail service. Production should set it to `true` and provide
`SAMEFRAME_MAIL_WEBHOOK_URL` plus `SAMEFRAME_MAIL_WEBHOOK_SECRET`; startup fails
closed if enforcement is enabled without delivery configuration.

## Playback protocol

Every server message uses this envelope:

```json
{
  "type": "playback.snapshot",
  "room": "ABC123",
  "seq": 42,
  "ts": 1791300000123,
  "from": "server",
  "payload": {}
}
```

- The owner is the only client allowed to submit shared playback controls.
- Client control sequence numbers reject replayed or out-of-order commands.
- Server sequence numbers let clients discard old snapshots.
- A full `room.state` is sent after every connection and after membership
  changes, so reconnects converge without replaying an event log.
- Playback controls execute in Redis Lua scripts, making projection, replay
  rejection, mutation, and sequence increment atomic across service nodes.
- A short Redis lock elects one node per room to emit each periodic snapshot,
  preventing duplicate snapshot streams in a multi-instance deployment.
- Presence uses 15-second heartbeats and a 45-second expiry window; multiple
  connections for one account are deduplicated in the displayed online count.
- Normal playback snapshots are broadcast every three seconds and immediately
  after a control command.
- Clients estimate server clock offset using the lowest-RTT of three samples.
- Drift over 1.5 seconds causes a seek; drift over 0.3 seconds uses 0.98x/1.02x
  rate correction; smaller drift restores the room speed.
- `source_version` and a client-side generation counter protect source reloads
  from stale asynchronous callbacks.

## Security decisions already enforced

- Media URLs containing embedded user credentials are rejected.
- The API never accepts or returns source headers, cookies, or provider tokens.
- Browser WebSockets use a room-scoped, 30-second, one-time ticket instead of a
  long-lived access token in the URL.
- Cross-origin HTTP and WebSocket access is allowlisted by configuration.
- Non-owner playback controls are rejected by the server.
- Request bodies and WebSocket messages have size limits.
- Administrator authorization is loaded from the database for every admin
  request; stale JWT claims cannot grant access and non-admin users are denied
  by default.
- Device bans revoke active user-device links and refresh tokens in one
  transaction. The next authenticated request fails immediately.
- Administrative mutations append a SHA-256 hash-chained audit record. Free-form
  report resolutions and ban reasons are deliberately excluded from audit
  metadata, and sensitive metadata key names are stripped before hashing.

The `*` origin setting in `docker-compose.yml` is for local development only.

## Next production slices

1. Move critical Pub/Sub events to Redis Streams if replay is needed and export
   audit-chain checkpoints to immutable external storage.
2. Add object-storage adapters behind the same encrypted source interface; move
   the static vault master key to a managed KMS envelope.
3. Embedded-track controls, subtitle style/delay, and richer cache controls.
4. Add moderator queues, appeals, retention controls, and abuse analytics before
   enabling public room discovery.
