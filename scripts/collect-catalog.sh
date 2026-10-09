#!/usr/bin/env bash
# Sent over SSH by Collect catalog; no script installation on the VDS is needed.
set -Eeuo pipefail
[[ $# -le 1 ]] || { echo 'Usage: collect-catalog.sh [DEPLOYMENT_DIRECTORY]' >&2; exit 1; }
base=${1:-/srv/devcourse-finder}
[[ $base == /* && -d $base ]] || { echo 'Deployment directory is missing.' >&2; exit 1; }
for command in docker flock readlink; do
  command -v "$command" >/dev/null || { echo "Missing $command" >&2; exit 1; }
done
[[ -f "$base/.env" && ! -L "$base/.env" ]] || { echo 'Production .env is missing or is a symlink.' >&2; exit 1; }
# Use the same host lock as release deployment before resolving current.
exec 9>"$base/.deploy.lock"
flock --nonblock 9 || { echo 'Deployment or manual collection is already running on this host. Retry after it finishes.' >&2; exit 1; }
[[ -d "$base/current" ]] || { echo 'Deploy a release with the catalog updater before collecting.' >&2; exit 1; }
release=$(readlink -f "$base/current")
[[ -f "$release/compose.yaml" && -f "$release/compose.production.yaml" ]] || { echo 'The deployed Compose files are missing.' >&2; exit 1; }
compose=(docker compose --project-name devcourse-finder --env-file "$base/.env" -f "$release/compose.yaml" -f "$release/compose.production.yaml")
services=$("${compose[@]}" config --services </dev/null)
[[ $'\n'"$services"$'\n' == *$'\ncatalog-updater\n'* ]] || { echo 'The deployed release does not include catalog-updater.' >&2; exit 1; }
container=$("${compose[@]}" ps --status running -q catalog-updater </dev/null)
[[ -n $container ]] || { echo 'The deployed catalog-updater is not running. Inspect its service logs.' >&2; exit 1; }
echo '[STEP] Discovering candidates and checking due records with the deployed worker'
result=0
# Disable exec stdin: bash itself is reading this script from the SSH stream.
"${compose[@]}" exec --interactive=false -T catalog-updater devcourse-finder catalog-update once </dev/null || result=$?
echo '[STEP] Collection status and measured coverage'
"${compose[@]}" exec --interactive=false -T catalog-updater devcourse-finder catalog-update status </dev/null || {
  if [[ $result -eq 0 ]]; then result=1; fi
}
exit "$result"
