#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
backup_directory="${BACKUP_DIR:-$PWD/backups}"
umask 077
mkdir -p "$backup_directory"
backup_file="$backup_directory/devcourse-$(date -u +%Y%m%dT%H%M%SZ).dump"
partial_file="$backup_file.partial"
trap 'rm -f "$partial_file"' EXIT
docker compose exec -T db sh -c 'pg_dump --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --format=custom' >"$partial_file"
test -s "$partial_file"
mv "$partial_file" "$backup_file"
# Retain the last seven days; only remove this script's completed dump files.
find "$backup_directory" -maxdepth 1 -type f -name 'devcourse-*.dump' -mtime +6 -delete
printf 'Backup completed: %s\n' "$backup_file"
