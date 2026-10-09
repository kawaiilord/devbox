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

## Emby lifecycle

1. The owner submits an HTTPS Emby API base URL, username, and password. The API
   base may include a reverse-proxy prefix such as `/emby/`.
2. SameFrame sends the password once to Emby's documented
   `POST /Users/AuthenticateByName` endpoint with a server-generated device ID.
3. Only the returned user ID, device ID, server ID, and access token are
   encrypted in the credential vault. The password is never persisted.
4. Library browsing calls `GET /Users/{UserId}/Items` and maps folders and video
   items into the same `MediaFile` tree used by WebDAV. Opaque item IDs form the
   virtual path; upstream filesystem paths are never exposed.
5. Ticket creation re-fetches the selected item, chooses its first valid media
   source, and stores the item/media-source/container tuple only inside the
   short-lived Redis ticket.
6. Playback uses Emby's static `/Videos/{Id}/stream.{Container}` endpoint through
   SameFrame's proxy. Text subtitle streams use Emby's documented subtitle
   endpoint and are requested as WebVTT.
7. Deleting an Emby source attempts `POST /Sessions/Logout` before destroying the
   encrypted local record.

SameFrame supports direct/static streams in this milestone. It intentionally
does not expose Emby tokens to the player and does not yet request server-side
transcoding or HLS. The implementation follows Emby's official
[user authentication](https://dev.emby.media/doc/restapi/User-Authentication.html),
[library browsing](https://dev.emby.media/doc/restapi/Browsing-the-Library.html),
[video streaming](https://dev.emby.media/doc/restapi/Video-Streaming.html), and
[subtitle](https://dev.emby.media/doc/restapi/Subtitles.html) contracts.

## Room playback tickets

Rooms persist `media_source_id` and a virtual `media_path`, never a provider
credential or long-lived playback URL. Any active room member may request a
five-minute opaque ticket. Redis stores the ticket mapping to the room owner's
source and validated upstream media identity.
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
stores WebDAV or Emby credentials because the upstream is already a short-lived
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
