# Room interaction and subtitles

## Chat

Room chat uses the existing WebSocket envelope with `type: chat.message`.
Clients submit only `{body}`; the server supplies the authenticated user ID,
display name, message ID, room code, and timestamp.

- Only persisted room members may send or read history.
- Messages are trimmed, must be valid UTF-8, reject NUL, and are limited to 500
  Unicode code points.
- Redis applies a cross-node limit of 20 messages per user per room per 10
  seconds.
- PostgreSQL assigns the canonical ID before Redis Pub/Sub broadcasts the
  message, so reconnecting clients can deduplicate and paginate reliably.
- `GET /api/v1/rooms/{code}/messages?before={id}&limit=50` returns ascending
  display order while querying the newest page efficiently.
- Blocking works in both directions: if either participant blocks the other,
  their messages are excluded from history and from local or cross-node realtime
  delivery. This check is server-side; the Flutter client also removes already
  displayed messages after a block for immediate visual feedback.
- Turning off `allow_room_chat` returns an empty history, rejects sends, and
  suppresses realtime delivery for that account.

## External subtitles

For vaulted media rooms, members can list text subtitle files in the video’s
own directory. Allowed extensions are SRT, WebVTT, ASS, and SSA. The server
rejects any subtitle path outside that directory, then issues a separate
15-minute media ticket.

The Flutter player loads the result with `SubtitleTrack.uri`. When the video
ticket renews or the player is rebuilt, the selected subtitle receives a fresh
ticket and is reapplied. Embedded tracks remain available through media_kit’s
automatic track selection.

## Playback failures

Player errors are classified into:

- expired/unauthorized ticket — renew automatically;
- network failure — retain room state and show reconnect guidance;
- decode/codec failure — suggest a different player or source;
- source missing — report that the media is unavailable;
- unknown — preserve the original diagnostic text.

This classification is client guidance, not an authorization decision. Server
HTTP status and room membership remain authoritative.
