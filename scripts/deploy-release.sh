#!/usr/bin/env bash
# Invoked over SSH by the manual Deploy release workflow.
set -Eeuo pipefail
[[ $EUID -eq 0 && $# -eq 3 ]] || { echo 'Usage (root): deploy-release.sh TAG SHA ARCHIVE_SHA256' >&2; exit 1; }
tag=$1
sha=$2
digest=$3
[[ $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ && $sha =~ ^[a-f0-9]{40}$ && $digest =~ ^[a-f0-9]{64}$ ]] || {
  echo 'Invalid release identifiers.' >&2; exit 1;
}
for command in docker jq curl flock sha256sum tar; do
  command -v "$command" >/dev/null || { echo "Missing $command" >&2; exit 1; }
done
base=/srv/devcourse-finder
[[ -f "$base/.env" && ! -L "$base/.env" ]] || { echo 'Production .env is missing or is a symlink.' >&2; exit 1; }
[[ ! -e "$base/current" || -L "$base/current" ]] || { echo 'The current release path is not a symlink.' >&2; exit 1; }
exec 9>"$base/.deploy.lock"
flock --wait 1800 9
umask 077
archive="$base/incoming/$tag-$sha.tar"
printf '%s  %s\n' "$digest" "$archive" | sha256sum --check --status
release="$base/releases/$tag-$sha"
temporary="$base/releases/.unpack-$tag-$$"
partial=''
link="$base/.current-$$"
record="$base/.DEPLOYED_VERSION-$$"
cleanup() {
  rm -rf "$temporary"
  rm -f "$link"
  rm -f "$record"
  if [[ -n $partial ]]; then rm -f "$partial"; fi
}
trap cleanup EXIT
trap 'echo "Deployment failed. The database is not automatically restored. Inspect the logs and the pre-deployment backup before retrying." >&2' ERR
mkdir -p "$base/releases" "$base/backups"
if [[ -e $release || -L $release ]]; then
  [[ -d $release && ! -L $release && $(cat "$release/.release-commit") == "$sha" ]] || {
    echo 'Existing release directory does not match the requested commit.' >&2; exit 1;
  }
else
  mkdir "$temporary"
  tar --extract --file "$archive" --directory "$temporary" --no-same-owner
  [[ "v$(cat "$temporary/VERSION")" == "$tag" && ! -e "$temporary/.env" ]]
  printf '%s\n' "$sha" > "$temporary/.release-commit"
  mv "$temporary" "$release"
fi
previous=$base
if [[ -L "$base/current" ]]; then previous=$(readlink -f "$base/current"); fi
old_compose=(docker compose --project-name devcourse-finder --env-file "$base/.env" -f "$previous/compose.yaml" -f "$previous/compose.production.yaml")
compose=(docker compose --project-name devcourse-finder --env-file "$base/.env" -f "$release/compose.yaml" -f "$release/compose.production.yaml")

"${compose[@]}" config --format json | jq -e '
  (.services.frontend.ports | length == 1) and
  (.services.frontend.ports[0].host_ip == "127.0.0.1") and
  (.services.frontend.ports[0].published | tostring == "3000") and
  ((.services.api.ports // []) | length == 0) and
  ((.services.db.ports // []) | length == 0) and
  (.services.api.environment.APP_ENV == "production")
' >/dev/null
docker volume inspect devcourse-finder_postgres-data >/dev/null
echo '[STEP] Backing up the existing database before migrations'
stamp=$(date -u +%Y%m%dT%H%M%SZ)
backup="$base/backups/before-$tag-$stamp-$$.dump"
partial="$backup.partial"
timeout --kill-after=5s 300 "${old_compose[@]}" exec --interactive=false -T db sh -c \
  'PGCONNECT_TIMEOUT=5 pg_dump --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --format=custom' </dev/null > "$partial"
test -s "$partial"
mv "$partial" "$backup"
partial=''
echo "Database backup: $backup"
echo "[STEP] Building and starting $tag ($sha)"
"${compose[@]}" up --build -d --remove-orphans --wait --wait-timeout 180 </dev/null
timeout --kill-after=2s 15 "${compose[@]}" exec --interactive=false -T api \
  wget -Y off -T 10 -qO- http://127.0.0.1:8080/health/ready </dev/null | jq -e '.ok == true' >/dev/null
ready=false
for attempt in {1..30}; do
  if curl --fail --silent --noproxy '*' --max-time 5 http://127.0.0.1:3000/api/v1/courses |
    jq -e '(.items | type == "array") and (.total | type == "number")' >/dev/null; then
    ready=true
    break
  fi
  sleep 2
done
$ready || { echo 'Frontend/API verification failed.' >&2; exit 1; }
[[ $(curl --silent --noproxy '*' --output /dev/null --write-out '%{http_code}' --max-time 15 http://127.0.0.1:3000/) == 200 ]]
printf 'version=%s\ncommit=%s\ndeployed_at=%s\n' "$tag" "$sha" "$(date -u +%FT%TZ)" > "$record"
ln -s "$release" "$link"
mv -Tf "$link" "$base/current"
mv -Tf "$record" "$base/DEPLOYED_VERSION"
rm -f "$archive"
echo "Deployment succeeded: $tag ($sha)"
echo 'Production .env, Nginx/HTTPS, database volume, and catalog data were preserved.'
