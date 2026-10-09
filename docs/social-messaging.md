# Social discovery and private messaging

User search respects `allow_profile_find` and excludes either direction of a
block. Follow edges are directional and profile counts are derived from the
authoritative relation table.

Conversation pairs are canonicalized so two users can have only one direct
conversation. Creating a conversation and every subsequent send checks the
recipient's `allow_private_chat` preference. Message history, conversation
lists, unread counts, and sends also deny access after either user blocks the
other.

Each user has a per-conversation `last_read_message_id` cursor. Sending a message
advances the sender's cursor transactionally; reading a page advances the
recipient cursor. This avoids destructive per-message read flags and supports
multiple devices.

`/ws/v1/social` uses the same 30-second one-time-ticket pattern as room sockets.
PostgreSQL is written before `social.message` and `social.unread` envelopes are
published through Redis. A disconnected client loads missed messages and unread
counts from PostgreSQL on reconnect, so Redis Pub/Sub is never the durable
message store.
