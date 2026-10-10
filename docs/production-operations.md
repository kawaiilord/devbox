# Production monitoring, backup, and retention

## Health and metrics

- `/healthz` is a process liveness probe and never checks dependencies.
- `/readyz` checks the repository and configured Redis with a two-second
  deadline. It returns 503 and `Retry-After: 5` when a required dependency is
  unavailable.
- `/metrics` exports a private Prometheus registry containing Go/process
  collectors, bounded-route request counts, status classes, latency histogram,
  in-flight requests, connected room clients, and rooms with live sockets.

Set `SAMEFRAME_METRICS_TOKEN` and send it as a Bearer token from Prometheus.
Keep the endpoint on a private network even with authentication. The supplied
Prometheus scrape configuration, alerts, and Grafana dashboard are under
`ops/monitoring/`.

Recommended Kubernetes probes:

```yaml
livenessProbe:
  httpGet: {path: /healthz, port: 8080}
readinessProbe:
  httpGet: {path: /readyz, port: 8080}
```

Route labels use registered Go patterns rather than raw URLs, preventing room,
order, and user identifiers from causing metric-cardinality growth. Structured
JSON request logs include a generated request ID, method, registered route,
status, and duration without query strings. They must be shipped with access
restricted; never log authorization
headers, passwords, source credentials, payment signatures, or Bot keys.

## PostgreSQL backups

`ops/backup/postgres-backup.sh` creates an atomic custom-format `pg_dump`,
validates its catalog, writes SHA-256, and removes files older than
`BACKUP_RETENTION_DAYS` (14 by default). Run it from a locked-down scheduled job
with a read-capable database credential and encrypted destination volume.

For production point-in-time recovery, also enable PostgreSQL WAL archiving to
immutable object storage. Keep daily logical backups for 14 days, weekly for 8
weeks, monthly for 12 months, and WAL long enough to span the oldest retained
base backup. Adjust these defaults to contractual and legal requirements.

Quarterly restore drill:

1. Provision an isolated PostgreSQL instance at the same major version.
2. Verify the dump checksum and run `pg_restore --list`.
3. Restore with `pg_restore --clean --if-exists --no-owner`.
4. Run migrations and the application readiness probe.
5. Verify user, room, order, audit-chain, complaint, and deletion-request counts.
6. Record RPO/RTO, failures, operator, and immutable evidence of the drill.

Backups contain personal data and encrypted credential envelopes. Encrypt them
with a distinct backup KMS key, restrict restore privileges, log access, and
destroy expired copies. Back up the credential-vault key separately; a database
dump without that key cannot restore provider credentials.

## Object storage and Redis

The review-image bucket should have versioning, server-side encryption,
replication, and lifecycle expiration for abandoned uploads. Database rows are
the authority for referenced keys. Test restoring both PostgreSQL and the bucket
to a consistent timestamp.

Redis contains reconstructable room coordination, rate limits, tickets, and
presence. It is not the system of record and does not require backup. Use a
managed highly available deployment, authentication/TLS, memory alerts, and a
policy that permits loss/rebuild from PostgreSQL after failover.

## Incident thresholds

The supplied rules alert on target loss, sustained 5xx ratio above 5%, p95
latency above one second, process RSS above 1 GiB, and prolonged absence of room
clients. Add environment-specific alerts for PostgreSQL replication lag,
connection saturation, disk/WAL growth, backup age/checksum failure, Redis
evictions, certificate expiry, complaint SLA breaches, and payment callback
failures. Every paging alert needs an owner and linked runbook.
