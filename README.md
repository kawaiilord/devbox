# SameFrame MVP

SameFrame is a clean-room synchronized viewing prototype based on the supplied
product and protocol blueprint. It does not contain code, branding, assets, or
credentials from the analyzed product.

The current foundation contains:

- a Flutter client for web, Linux, and Windows;
- a Go HTTP/WebSocket service;
- server-authoritative rooms with owner-only playback controls;
- periodic playback snapshots, sequence checks, and reconnect recovery;
- a `media_kit` (libmpv family) player adapter;
- clock-offset measurement and hard/soft playback alignment;
- email/password accounts with Argon2id password hashing;
- 15-minute JWT access tokens and rotating, revocable refresh tokens;
- PostgreSQL persistence for users, rooms, members, and playback state;
- Redis atomic hot-room state, distributed sequencing, Pub/Sub, presence, and
  one-time WebSocket tickets;
- persistent device identities with access-token binding and remote revocation;
- Redis-backed cross-node limits for registration, login, refresh, verification,
  and password recovery;
- single-use email verification and password-reset tokens delivered through a
  configurable authenticated webhook;
- encrypted WebDAV sources, renewable five-minute media tickets, Range
  streaming, and a bounded desktop loopback cache;
- tests and reproducible local/container commands.

## Run locally

Start PostgreSQL and the server:

```bash
docker compose up --build
```

In a GitHub Codespace whose Docker bridge is restricted, use:

```bash
docker compose -f docker-compose.yml -f docker-compose.codespace.yml up --build
```

In another terminal, start the Flutter web client:

```bash
cd client
flutter run -d web-server \
  --dart-define=API_BASE_URL=http://localhost:8080
```

Open two browser windows, create a room in one, and join it from the other.
Use a direct HTTP(S) media URL that permits cross-origin playback.

## Verify

```bash
make test
make build
```

`make build` produces the Go server binary and a Flutter web release bundle.
Windows binaries must be produced on a Windows runner; the workflow in
`.github/workflows/ci.yml` verifies source and web compilation on Linux.

## Current boundary

This is an engineering foundation, not a production release. Production work
still requires secure client credential storage, a production mail provider,
device-ban administration, a credential vault, TURN, signed updates,
moderation, and payment integration.
Third-party source credentials are never accepted by this milestone.

See `docs/architecture.md` for the topology, `docs/account-security.md` for
identity controls, and `docs/media-sources.md` for the credential and playback
data flow.
