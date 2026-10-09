# Moderation, privacy, and audit controls

## User controls

Authenticated users can read and patch `/api/v1/privacy`. The three explicit
settings are room chat participation, profile discovery, and watch-activity
visibility. Settings default to enabled and partial PATCH requests preserve
unspecified values.

`/api/v1/blocks/{user_id}` creates or removes a one-way block preference. Chat
enforcement is intentionally bidirectional: a message is hidden when either the
viewer blocked the sender or the sender blocked the viewer. Both WebSocket
delivery and paginated history enforce this rule on the server.

Room, message, and user reports are accepted at `/api/v1/reports`. The server
validates target/reason lengths, limits free-form details to 1,000 Unicode code
points, and applies a Redis-backed per-user submission limit when Redis is
configured.

## Administrator boundary

Set `SAMEFRAME_ADMIN_EMAILS` to a comma-separated allowlist. Matching new
registrations are provisioned as administrators; matching existing accounts are
reconciled at startup. This setting must be supplied by deployment secrets or
trusted configuration, never by a client. Administrator endpoints additionally
require the account's email to be verified, so allowlisting an address does not
grant control before the operator-provided mail flow proves ownership.

Every `/api/v1/admin/*` request authenticates the device and reloads the account
from the repository. Access is denied unless the current database row has
`is_admin=true`. Administrators can:

- list and resolve pending reports;
- close a room, which pauses it and prevents join, resume, ticket, chat, and
  playback operations across nodes;
- ban a SHA-256 device identifier, immediately revoking its active links and
  refresh tokens;
- inspect the paginated audit chain and its page-integrity result.

## Audit chain

Report resolutions, room closures, and device bans append an audit event. The
event hash covers the previous hash, actor, normalized action and target,
sanitized metadata, and server timestamp. PostgreSQL serializes append
transactions with an advisory transaction lock so concurrent administrators
cannot fork the chain. Each PostgreSQL moderation mutation and its audit event
commit in the same transaction; neither can persist without the other.

Audit metadata is bounded and recursively sanitized. Keys suggesting passwords,
tokens, cookies, credentials, authorization headers, private/API keys, or other
secrets are removed. Free-form report and ban text is not copied into the audit
record. The database chain is tamper-evident rather than immutable; production
deployments should periodically export signed checkpoints to write-once storage.
