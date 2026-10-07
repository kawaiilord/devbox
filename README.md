# SameFrame MVP

SameFrame is a clean-room synchronized viewing prototype based on the supplied
product and protocol blueprint. It does not contain code, branding, assets, or
credentials from the analyzed product.

The first milestone contains:

- a Flutter client for web, Linux, and Windows;
- a Go HTTP/WebSocket service;
- server-authoritative rooms with owner-only playback controls;
- periodic playback snapshots, sequence checks, and reconnect recovery;
- a `media_kit` (libmpv family) player adapter;
- clock-offset measurement and hard/soft playback alignment;
- tests and reproducible local/container commands.

## Run locally

Start the server:

```bash
cd server
go run ./cmd/sameframe
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

This is an engineering foundation, not a production release. Demo sessions and
rooms are held in memory. Production work still requires persistent users,
PostgreSQL/Redis, a credential vault, TURN, signed updates, rate limiting,
moderation, and payment integration. Third-party source credentials are never
accepted by this milestone.
