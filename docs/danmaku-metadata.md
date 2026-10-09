# Danmaku and metadata

## Source matching

SameFrame derives a 64-character SHA-256 fingerprint from the room's source
identity and episode number. Vaulted sources use source ID plus virtual path;
direct rooms hash the URL without persisting it in the danmaku table. This lets
different rooms playing the same media reuse one timeline while keeping source
details out of responses.

Messages contain authenticated user/display identity, body, playback position,
RGB color, mode, and server timestamp. The server:

- requires room membership and room-chat privacy permission;
- accepts 1–100 visible characters and three rendering modes;
- rejects positions more than 30 seconds from authoritative playback;
- persists before local or Redis cross-node broadcast;
- rate-limits each room/user pair when Redis is enabled;
- filters history and realtime delivery when either user blocked the other.

The Flutter video stack renders timeline-aligned scrolling/top/bottom messages.
Users can disable danmaku or add local keyword filters without affecting other
room members.

## Metadata search

When `SAMEFRAME_TMDB_TOKEN` is configured, authenticated clients can search
movies and TV shows through SameFrame. The API token remains server-side and is
sent as a Bearer header according to TMDB's official
[application authentication](https://developer.themoviedb.org/docs/authentication-application)
and [search workflow](https://developer.themoviedb.org/docs/search-and-query-for-details).

The proxy fixes the upstream host, blocks cross-authority redirects, disables
adult results, limits response size and result count, and returns a strict
projection of title, overview, release date, rating, and public poster URL.
Person results and arbitrary upstream fields are discarded.
