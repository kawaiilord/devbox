# Room voice

The Flutter client uses `flutter_webrtc` for audio-only peer connections on Web,
Linux, and Windows. Joining voice explicitly requests microphone permission;
incoming signaling never activates a microphone before the user joins.

Each room member forms a mesh connection with other joined voice users. A
reciprocal `ready` handshake elects the lower user ID to send one offer. Incoming
SDP/ICE operations are serialized and early ICE is queued until a remote
description exists. Remote audio tracks are attached to an initialized
`RTCVideoRenderer`; merely creating a peer connection does not play audio on the
web. If browser autoplay is blocked, the voice dialog offers an explicit
"开启声音" action. Offer, answer, candidate, mute, and hangup payloads are size/enum
validated by the server, membership checked, and routed only to
`target_user_id`. They are ephemeral and never persisted.

`SAMEFRAME_TURN_URLS` and `SAMEFRAME_TURN_SECRET` configure TURN. The API returns
a username containing expiry plus user ID and an HMAC-SHA1 credential, following
the common TURN REST pattern. Credentials expire after one hour. The shared
secret is never sent to clients or logs.

The dialog reports actual connected peers, microphone acquisition progress and
actionable errors. Local microphone tracks stop on hangup, room disposal,
permission revocation, room closure, or signaling disconnection. Late hangup
messages are ignored after the room socket closes; they cannot prevent cleanup.

Without TURN configuration the voice feature is advertised as unavailable and
the room config endpoint fails closed. `ops/releases/room-voice.py` provisions
coturn with shared-secret authentication, quotas, private-network peer denies,
and TLS using the existing site certificate. It adds a Compose override and an
owner-only `turn/app.env`, and extends the existing certificate renewal hook.
Only app and TURN services are affected by this release. The combined rollback
removes its override and TURN service and restores the previous app/web/renewal
configuration without restoring an old database.

The relay host must be publicly reachable on UDP/TCP 3478, TLS/TCP 5349 and its
configured UDP relay range (49160–49359). A server behind NAT requires the
public/private address mapping. TCP/TLS client transports support networks that
block client UDP. Use `turns:` with a hostname matching the certificate; an HTTP
reverse proxy or a proxied DNS record does not forward TURN traffic.

The loopback-only acceptance fixture can enable voice with `--voice` and the
same TURN environment variables. Synthetic microphones and isolated accounts
can then verify real relay candidates, received RTP, nonzero decoded audio
energy, actual audio-element playback, mute and capture release without
creating production test accounts.
