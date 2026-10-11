#!/bin/sh
set -eu
cd /opt/sameframe
before=$(find letsencrypt/live -name fullchain.pem -exec readlink {} \; | sort)
docker run --rm -v /opt/sameframe/letsencrypt:/etc/letsencrypt -v /opt/sameframe/acme:/var/www/acme certbot/certbot:v5.4.0 renew --non-interactive --quiet
after=$(find letsencrypt/live -name fullchain.pem -exec readlink {} \; | sort)
if [ "$before" != "$after" ]; then
  docker compose exec -T web nginx -t
  docker compose exec -T web nginx -s reload
  docker compose restart mediamtx
fi
