# Couple space

Binding starts with a directed request and requires the requester to have an
active membership both when requesting and when the recipient accepts. Pair
creation serializes both user IDs so concurrent accepts cannot give one account
multiple active partners.

An accepted pair stores its anniversary and a durable event timeline. Either
partner can publish shared moments. Separation does not delete data: it starts a
seven-day cooling period, during which either partner can restore the relation.
After cooling expires, restore is denied.

Shared favorites and common watch history are computed from each partner's
private library and return only intersecting titles. Provider credentials,
virtual paths belonging to the partner, and playback URLs are never exposed.
Deleting either account cascades requests, relationship records, moments, and
timeline events through PostgreSQL foreign keys.
