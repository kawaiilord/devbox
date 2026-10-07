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
still requires Redis-backed multi-instance fan-out and presence, secure client
credential storage, email verification, account recovery, a credential vault,
TURN, signed updates, rate limiting, moderation, and payment integration.
Third-party source credentials are never accepted by this milestone.
