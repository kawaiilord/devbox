#!/usr/bin/env sh
set -eu
: "${DATABASE_URL:?DATABASE_URL is required}"
: "${BACKUP_DIR:?BACKUP_DIR is required}"
retention_days="${BACKUP_RETENTION_DAYS:-14}"
case "$BACKUP_DIR" in
  /|/home|/root|/var) echo "BACKUP_DIR must be a dedicated subdirectory" >&2; exit 1 ;;
esac
case "$retention_days" in
  ''|*[!0-9]*) echo "BACKUP_RETENTION_DAYS must be a non-negative integer" >&2; exit 1 ;;
esac
umask 077
mkdir -p "$BACKUP_DIR"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
partial="$BACKUP_DIR/sameframe-$timestamp.dump.partial"
final="$BACKUP_DIR/sameframe-$timestamp.dump"
pg_dump --format=custom --no-owner --no-acl --file="$partial" "$DATABASE_URL"
pg_restore --list "$partial" >/dev/null
mv "$partial" "$final"
sha256sum "$final" > "$final.sha256"
find "$BACKUP_DIR" -type f -name 'sameframe-*.dump' -mtime "+$retention_days" -delete
find "$BACKUP_DIR" -type f -name 'sameframe-*.dump.sha256' -mtime "+$retention_days" -delete
printf '%s\n' "$final"
