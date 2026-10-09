# Favorites and watch history

## Favorites

Authenticated users can save a file from one of their own WebDAV or Emby
sources. Before an upsert, the server validates source ownership and asks the
source adapter to validate the virtual media path. Favorites are unique by user,
source, and path, so repeated saves update metadata instead of creating
duplicates.

Deleting a media source cascades its favorites. No provider token, password,
upstream filesystem path, or playback URL is stored in a favorite.

## Authoritative watch progress

While a room is open, the Flutter client sends its media duration every 30
seconds and once during teardown. The client does not submit a playback
position. The server verifies room membership and records the projected
authoritative room position, episode, completion state, and companion count.
Completion is monotonic, and the stored companion count is the maximum observed
for that media identity.

The media identity is a SHA-256 digest:

- source-backed rooms hash source ID plus virtual media path;
- direct-link rooms hash the source URL, without persisting that URL in history.

For a source-backed room, only the source owner receives a resumable source ID
and path. Other room members receive a non-resumable history entry and therefore
cannot reuse the owner's source or credentials.

## Resume behavior

The private library page can create a new room from a favorite or resumable
history record. A resumed room starts from the stored server-authoritative
position. If the media source has been deleted, PostgreSQL keeps the historical
title and progress but marks the record non-resumable.

## Public activity and privacy

`GET /api/v1/users/{id}/watch-activity` is a deliberately reduced projection. It
contains title, episode, completion state, companion count, and watch timestamp
only. The server denies the request when `show_watch_activity` is disabled or
when either account has blocked the other. Private history remains visible only
to its owner.
