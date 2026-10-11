# Platform services

This round extends the existing monochrome web console and Flutter client.
Migration 019 adds approval queues, security ceremonies, external identity links
and room stream keys. All settings use optimistic versions and audit events.

- Registration, room-creation and room-join reviews have pending/approved/rejected
  states. Pending registration has no session. Pending rooms are hidden. Room
  owners can review joins; administrators can review all kinds. Stale decisions
  conflict; rejecting a stale request cannot undo a ban. Client pages expose
  personal requests and room join queues. Pending rooms count toward room limits.
- SMTP supports TLS, required STARTTLS, local fixtures, sender, templates and an
  explicit test-mail button. Secrets are encrypted; verification/reset links use
  the configured origin and a fragment token. Webhook delivery remains supported.
- Password plus TOTP and single-use recovery codes; TOTP replay prevention;
  email-code login for verified active users; GitHub OAuth and standard OIDC with
  state, browser cookie binding, PKCE, nonce, signature, issuer and audience
  verification. Matching an email never silently links an existing account.
  WebAuthn uses go-webauthn, requires user verification, stores credentials and
  counters, and checks ceremony origin/expiry. Security changes revoke sessions.
  Native clients can finish browser-based login using a short-lived handoff;
  no access token is included in a URL. `/admin/account.html` is the security UI.
- Remote gRPC Provider connections support endpoint/TLS/custom CA/timeout/
  enable/disable/reconnect/health. Wire messages are copied under the upstream MIT
  license from SyncTV server ca91048b9da595e50642a61618b0eabfbc05e09b. The actual
  remote transport supports Alist, Emby and Bilibili. It sends `x-provider-secret`
  metadata. Alist login/browse/file resolution, Emby login/items/playback, and
  Bilibili episode/DASH quality resolution are connected to room playback. This
  is not a claim of compatibility with every SyncTV application RPC.
- RTMP uses MediaMTX 1.21.2. Room owners generate expiring hashed publish keys,
  rotate/revoke keys and automatically select the live entry in the playlist.
  The MediaMTX auth hook validates owner/room/key state. Private HLS is accessed
  by the backend and rewritten to room-authorized encrypted asset links. The
  console lists connections and disconnects/revokes streams. A watchdog removes
  expired/revoked/closed-room publishers within its polling interval.
- Per-node bounded disk caching of small authorized range responses and HLS
  assets, TTL/LRU eviction, metrics and global/per-node purge generations. Redis
  stores expiring node reports. Configuration is shared by PostgreSQL. Cache keys
  include owner/source/credential and source versions. Source access is checked
  before cache lookup. Manifests and live RTMP are not cached. This cache stores
  up to 8 MB objects; it is not a general distributed object store.

Real SMTP delivery, external OAuth providers, a physical authenticator and real
third-party media accounts still require operator-specific configuration and
verification. Protocol fixtures do not prove that an arbitrary YouTube or Quark
account can play every video. Media P2P, general server transcoding, imported
platform danmaku, chapters/storyboards and native mobile runners remain separate
playback parity work. OPAQUE authentication is not implemented.
