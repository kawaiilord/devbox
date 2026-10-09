# Room voice

The Flutter client uses `flutter_webrtc` for audio-only peer connections on Web,
Linux, and Windows. Joining voice explicitly requests microphone permission;
incoming signaling never activates a microphone before the user joins.

Each room member forms a mesh connection with other joined voice users. A
`ready` signal repairs the case where an offer arrived before the other user
joined. Offer, answer, candidate, mute, and hangup payloads are size/enum
validated by the server, membership checked, and routed only to
`target_user_id`. They are ephemeral and never persisted.

`SAMEFRAME_TURN_URLS` and `SAMEFRAME_TURN_SECRET` configure TURN. The API returns
a username containing expiry plus user ID and an HMAC-SHA1 credential, following
the common TURN REST pattern. Credentials expire after one hour. The shared
secret is never sent to clients or logs.

Without TURN configuration the voice feature is advertised as unavailable and
the room config endpoint fails closed. Deployments should use TLS `turns:` URLs,
restrict relay quotas, and monitor allocation abuse.
