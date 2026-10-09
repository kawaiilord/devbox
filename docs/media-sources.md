# Media source security and playback

## WebDAV lifecycle

1. The authenticated owner submits a name, HTTPS base URL, username, and
   password.
2. The server validates DNS results and rejects loopback, private, link-local,
   multicast, unspecified, and carrier-grade NAT addresses by default.
3. Credentials are serialized and encrypted with AES-256-GCM. The source owner
   and source ID are authenticated as additional data, so ciphertext copied to
   another record cannot be decrypted.
4. PostgreSQL stores only the authenticated ciphertext. List and room APIs omit
   credentials completely.
5. Directory browsing uses WebDAV `PROPFIND` with `Depth: 1` and parses the 207
   Multi-Status response with an 8 MiB response limit.

Private source addresses and plain HTTP can be enabled only with
`SAMEFRAME_ALLOW_PRIVATE_SOURCES=true`; this is intended for isolated local
development networks, not an internet-facing deployment.

## Room playback tickets

Rooms persist `media_source_id` and `media_path`, never a provider credential or
long-lived playback URL. Any active room member may request a five-minute opaque
ticket. Redis stores the ticket mapping to the room owner's source and path.
The public `/media/{ticket}` endpoint:

- accepts only GET and HEAD;
- forwards only `Range` and `If-Range` request headers;
- returns only a small response-header allowlist;
- never forwards cookies, authorization headers, or upstream error bodies;
- streams 200/206 responses without buffering the entire video.

The client renews a room ticket every four minutes and rebuilds the player at
the current position without leaving the room.

## Desktop Range cache

Desktop builds bind an HTTP server to a random `127.0.0.1` port. The player
requests this local URL instead of the remote ticket URL. Each successful 200
or 206 response up to 64 MiB is cached by upstream URL + Range + If-Range. The
cache is capped at 512 MiB with oldest-entry eviction. Web builds use the
upstream URL directly.

The local route contains a random UUID and accepts only GET/HEAD. It never
stores WebDAV credentials because the upstream is already a short-lived
SameFrame media ticket.

## Configuration

```env
SAMEFRAME_VAULT_KEY=<base64 encoded 32-byte key>
SAMEFRAME_ALLOW_PRIVATE_SOURCES=false
SAMEFRAME_PUBLIC_BASE_URL=https://api.example.com
```

The vault key must come from a secret manager and remain stable across service
restarts. Losing or rotating it without a migration makes existing credential
records unreadable.
