# SameFrame web deployment

Copy this directory to a private directory on the target host. Supply a static
Linux binary at `bin/sameframe` and the Flutter release web artifact at `web/`.
Use a mode-0600 `.env` with randomly generated `POSTGRES_PASSWORD`,
`REDIS_PASSWORD`, `JWT_SECRET`, `RTMP_INTERNAL_SECRET`, and a base64-encoded
32-byte `VAULT_KEY`. Set `ADMIN_EMAIL`, `PUBLIC_URL`, `PUBLISH_URL`, and
`CERT_NAME`. Never commit that file. Keep the vault key in backups: stored source,
SMTP, MFA, OAuth and Provider secrets depend on it.

`PUBLIC_URL` must be the HTTPS origin. Flutter uses the serving origin when
`API_BASE_URL` was not specified at build time. `PUBLISH_URL` ends with
`/sameframe` (for example `rtmps://your-host:1936/sameframe`). Mount a valid TLS
certificate under `letsencrypt/live/CERT_NAME/` and render `nginx.conf.template`
as `nginx.conf`, replacing `CERT_NAME`.

Run `docker compose up -d --build`. PostgreSQL and Redis are isolated named
volumes with loopback-only maintenance ports 55433 and 56380. MediaMTX's control
API and HLS port are reachable only within this Compose network. Only authorized
room viewers receive signed SameFrame HLS asset links. Ports 80, 443, 1935 and
1936 must be available. No existing Docker resources should be removed.

The configured administrator email must register, then have its email verified
through the configured SMTP service or an operator bootstrap step. For a new
site, create a random initial password and mark only that specific bootstrap
account verified through an authenticated database operation. Store the temporary
credentials separately and replace the password after first login.

SMTP and external identity credentials are entered through the admin console.
Passkey needs an appropriate HTTPS RP domain (localhost is supported for tests).
Third-party platforms and private NAS connections need real credentials and
network access. The remote Provider transport supports upstream Alist/Emby/Bili
RPCs; configuring a Provider is not equivalent to making all SyncTV features work.

IP certificates can be obtained using Certbot >=5.4 with `--ip-address` and
`--preferred-profile shortlived`. They last six days: schedule renewals at least
twice daily, then reload nginx and MediaMTX after a changed certificate. Preserve
port 80's ACME challenge route. A domain is preferable for external login apps.

Back up PostgreSQL with `pg_dump` and back up `.env` and certificate account data
separately. Test recovery in an isolated database. Never run the destructive
integration tests against the application database; they require a separate
`sameframe_test` database and Redis DB 15.

For rate limiting behind nginx, set `TRUSTED_PROXY_CIDRS` to this deployment's
Docker network subnet. The server accepts `X-Real-IP` only from that subnet;
nginx overwrites the header. Keep the application maintenance port bound to
loopback. The nginx media proxy must include `/media/`, alongside `/api/`, `/ws/`
and `/admin/`, or encrypted media URLs will incorrectly reach the Flutter SPA.

Install `sameframe-tls.service` and `sameframe-tls.timer` under systemd and put
`renew-tls.sh` in `/opt/sameframe`. Enable the timer after the first certificate is
issued. Successful certificate replacement reloads nginx and restarts MediaMTX;
active publishers may briefly disconnect during certificate renewal.
