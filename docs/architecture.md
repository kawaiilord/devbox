# SameFrame architecture

## Clean-room boundary

This repository implements the ideas in the supplied blueprint with original
code and a new name and interface. It does not call the analyzed service, copy
its assets, or accept third-party cookies and media-library credentials.

## Milestone 1 topology

```text
Flutter client ── HTTPS ──► Go API
       │                      │
       ├── WebSocket ticket ──┤
       ├── WSS room channel ◄─┤── authoritative in-memory room state
       └── direct HTTP media  │
```

The current in-memory store is intentionally replaceable. The production
topology will put durable entities in PostgreSQL, hot room state and sequence
numbers in Redis, and WebSocket fan-out behind Redis Pub/Sub or Streams.

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

1. PostgreSQL migrations for users, rooms, members, refresh tokens, and audit
   records; Redis for snapshots, presence, and fan-out.
2. Password/email authentication, secure refresh-token rotation, device IDs,
   rate limits, and revocation.
3. Source adapters beginning with direct URLs and WebDAV. Provider credentials
   go into a KMS-backed vault; clients receive short-lived media tickets.
4. Local Range-aware cache proxy, subtitles, error classification, and source
   renewal.
5. Chat, moderation, reporting, and privacy controls before public rooms.
