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

The `*` origin setting in `docker-compose.yml` is for local development only.

## Next production slices

1. Device IDs, request rate limits, email verification, recovery, and audit
   records; move critical Pub/Sub events to Redis Streams if replay is needed.
2. Source adapters beginning with direct URLs and WebDAV. Provider credentials
   go into a KMS-backed vault; clients receive short-lived media tickets.
3. Local Range-aware cache proxy, subtitles, error classification, and source
   renewal.
4. Chat, moderation, reporting, and privacy controls before public rooms.
