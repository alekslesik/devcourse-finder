#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
# Own project/database/images only; do not accept an existing deployment name.
export COMPOSE_PROJECT_NAME="devcourse-e2e-$$"
export E2E_API_IMAGE="$COMPOSE_PROJECT_NAME-api"
export E2E_WEB_IMAGE="$COMPOSE_PROJECT_NAME-web"
export POSTGRES_DB=devcourse_e2e_test POSTGRES_USER=devcourse POSTGRES_PASSWORD=e2e-test-only
export API_PORT=8091 WEB_PORT=3091 SITE_URL=http://127.0.0.1:3091 CATALOG_OPERATOR=e2e-test
export E2E_REAL_API=1
compose=(docker compose -f compose.yaml -f compose.e2e.yaml)
workspace=$(mktemp -d)
cleanup() {
  result=$?
  if ((result != 0)); then "${compose[@]}" logs --no-color --tail=100 api frontend db >&2 || true; fi
  "${compose[@]}" down --volumes --remove-orphans || true
  docker image rm "$E2E_API_IMAGE" "$E2E_WEB_IMAGE" >/dev/null 2>&1 || true
  rm -rf "$workspace"
}
trap cleanup EXIT
build_args=()
if [[ -n "${CODEX_PROXY_CERT:-}" ]]; then
  build_args+=(--secret "id=proxy_ca,src=$CODEX_PROXY_CERT")
fi
docker build "${build_args[@]}" -t "$E2E_API_IMAGE" backend
docker build "${build_args[@]}" -t "$E2E_WEB_IMAGE" frontend
python3 scripts/e2e/catalog.py "$workspace/catalog.json"
chmod 644 "$workspace/catalog.json"
"${compose[@]}" up -d --no-build db migrate
"${compose[@]}" run --rm --no-deps -v "$workspace/catalog.json:/data/catalog.json:ro" api catalog validate /data/catalog.json
# run waits for the migration dependency before import.
"${compose[@]}" run --rm -v "$workspace/catalog.json:/data/catalog.json:ro" api catalog import /data/catalog.json
"${compose[@]}" up -d --no-build api frontend
for attempt in {1..60}; do
  if curl -fsS http://127.0.0.1:3091/about >/dev/null; then break; fi
  if [[ "$attempt" == 60 ]]; then exit 1; fi
  sleep 1
done
cd frontend
npm run test:e2e
