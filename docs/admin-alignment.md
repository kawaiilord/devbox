# Admin management alignment

See [platform services](platform-services.md) for the subsequent approvals, auth,
remote Provider, RTMP and cache round. The notes below describe the earlier round.

This extends the existing SameFrame server and monochrome `/admin/` console.
The reference is SyncTV App `1493e664b991a0f439614396d26bc2c18d692df6` and
SyncTV server `ca91048b9da595e50642a61618b0eabfbc05e09b`. It does not implement
SyncTV protobuf/gRPC wire compatibility.

## Implemented workflows

- **Users:** searchable pagination and status filters; create ordinary accounts,
  rename, reset passwords, change roles, ban/unban, move to a recycle bin and
  restore. Batch actions return an individual result for every selected account.
  Self-management of sensitive account state is denied. Administrators can
  manage ordinary accounts; super administrators can manage other staff.
- **Rooms:** global search, pagination, state filters, create direct-link rooms,
  rename, capacity, public/private visibility, password keep/set/clear, category,
  tags, guests, automatic next episode, member add/remove/unban, roles and
  permission overrides, ownership transfer, play/pause/seek/speed, ban/unban,
  recycle/restore, and batch lifecycle operations. Removing a member also
  prevents rejoining until explicitly unbanned. Expired rooms cannot be restored.
- **Taxonomy:** create/edit/enable/disable/delete category and tag definitions,
  parent-category assignment, ordering, and a public enabled-only catalog at
  `/api/v1/room-taxonomy`. The admin room editor consumes this catalog. Existing
  room category/tag values are preserved when a dictionary entry is removed.
  Client free-text category/tag fields continue to work.
- **Media services:** searchable inventory of sources bound by users, ownership
  metadata, rename, disable/enable, delete and connection probes. Probes browse a
  source root or resolve an explicitly provided platform video URL; they perform
  real provider requests. Platform parser availability is distinct from a
  successful account/connection probe. Secrets and upstream error text are not
  returned. This manages native SameFrame source bindings, not remote SyncTV
  gRPC Provider instances.
- **Site policy:** registration, room creation, guest access and chat switches;
  per-user active-room limit; default and maximum room capacity; chat-history
  page-size limit. These controls are enforced in server request paths. Limits
  apply to future room creation/settings; reducing capacity does not evict members.
  The history limit controls response size, not automatic retention/deletion.
- **Permissions:** the console retrieves current permissions before rendering its
  navigation. Operators can inspect rooms/users without management controls;
  sellers receive their order/code tools. Direct API calls remain permission checked.

## State and persistence

Migration `018_admin_management.sql` adds account state/revisions, source
state/revisions, room administrative epochs and versioned settings documents.
Both memory and PostgreSQL repositories support these workflows. PostgreSQL
state changes and their audit events commit in the same transaction. Edits use
optimistic versions; stale edits return HTTP 409. Batch operations are individually
transactional, not all-or-nothing, and do not silently ignore failed targets.

Account recycle-bin deletion disables login and retains related records and room
history. It is distinct from the existing irreversible user-requested account
closure workflow. Room access can be restricted independently in room management.
Restoring an account explicitly enables it; old access/refresh sessions stay invalid.
Passwords never appear in audit metadata. Issuing refresh sessions checks the
account version under a repository lock to fence concurrent password/status changes.

Bans, deletion and password changes revoke refresh/action tokens, increment the
session version and disconnect live room/social connections. Redis tickets carry
session versions; live connections are also periodically revalidated. Media tickets
check their viewer's current account/session state when a new request begins.
Source disablement blocks new browsing, ticket creation and proxy requests; it
cannot retract bytes already delivered or a third-party direct URL.

Room changes use the existing distributed room lock, database feature revision,
Redis state and WebSocket broadcasts. A persisted administrative epoch prevents
old snapshots from undoing a ban or a deliberate restore.

Upgrade **all backend nodes** together. Older binaries do not enforce the new
account states or administrative epochs. Retain the existing database, vault and
JWT configuration. No deployment is performed by producing the build artifacts.

## Remaining differences

The following are not supplied by this alignment increment:

- Registration, room-creation and join-approval queues.
- Remote gRPC Provider-instance CRUD and reconnection semantics used by SyncTV.
- SMTP/OAuth2/passkey/MFA configuration editors and their missing authentication
  mechanisms; these are not created by the new site-policy switches.
- RTMP publishing-session administration and distributed media slice-cache
  inventory/eviction. SameFrame's desktop HTTP cache is a different subsystem.
- Full SyncTV room/media moderation parity, including its resource-retention and
  cascading-delete model. This release uses the explicit recycle-bin behavior above.

Tests cover both repositories, role boundaries, stale writes, lifecycle actions,
credential/session invalidation, source disablement, redaction, policy enforcement,
taxonomy publication and preservation of the existing playback workflows.
