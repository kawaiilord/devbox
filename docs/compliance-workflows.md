# Account deletion and copyright complaints

## Account deletion

Authenticated users submit a deletion request with an explicit confirmation.
They may cancel while it remains pending. Administrators cannot delete their
own privileged account through this flow; the role must first be removed by a
different super administrator.

Approval requires a second irreversible confirmation in the independent admin
console. One serializable PostgreSQL transaction then:

- locks the request and target account;
- detaches payment orders and stores a one-way pseudonymous reconciliation ref;
- deletes owned rooms and cascades room messages/members;
- removes refresh tokens, device links, media credentials, favorites, history,
  reviews, direct messages, follows, blocks, points, and social/couple data;
- nulls optional operator references while retaining audit and financial rows;
- deletes the user and marks the request executed.

The running room state is immediately marked closed and broadcast after the
database commit. Existing access tokens stop working because authentication
reloads the deleted account on every request.

## Copyright complaints

`POST /api/v1/copyright-complaints` accepts authenticated or public claims and
is rate-limited by source address. Required fields include claimant identity,
contact email, a detailed rights basis, infringement URL, electronic signature,
and an accuracy declaration. Up to 20 HTTPS evidence links and an optional room
code are accepted.

Each claim receives a server-side `due_at` 24 hours after submission. The admin
queue sorts unresolved claims by deadline. Reviewers can triage, reject, or
action a claim; action may atomically record the resolution and immediately
close/broadcast the referenced room. Every resolution is appended to the
administrator audit chain.

This workflow records and enforces an operational response target; it does not
replace jurisdiction-specific legal review, identity verification, or evidence
retention policy.
