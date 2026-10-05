#!/usr/bin/env bash
set -euo pipefail

project_name="${COMPOSE_PROJECT_NAME:-devcourse-finder-smoke-${GITHUB_RUN_ID:-$$}}"
api_url="${SMOKE_API_URL:-http://127.0.0.1:8080}"
catalog_file="${SMOKE_CATALOG_FILE:-$PWD/data/demo-catalog.json}"

for command in docker curl python3; do
  command -v "$command" >/dev/null || {
    echo "Required command is not installed: $command" >&2
    exit 127
  }
done
docker compose version >/dev/null

compose=(docker compose --project-name "$project_name")

cleanup() {
  "${compose[@]}" down --volumes --remove-orphans
}
trap cleanup EXIT

wait_for_ready() {
  local attempts=60

  until curl --fail --silent --show-error "$api_url/health/ready" >/dev/null; do
    attempts=$((attempts - 1))
    if ((attempts == 0)); then
      echo "API did not become ready" >&2
      "${compose[@]}" ps >&2
      "${compose[@]}" logs api db migrate >&2
      return 1
    fi
    sleep 2
  done
}

assert_catalog() {
  local response slug offer_id detail comparison

  response="$(curl --fail --silent --show-error \
    "$api_url/api/v1/courses?language=go&page_size=48")"
  read -r slug offer_id < <(python3 -c '
import json, sys
payload = json.load(sys.stdin)
assert payload["total"] >= 5, payload
assert payload["items"], payload
item = payload["items"][0]
print(item["course"]["slug"], item["offer"]["id"])
' <<<"$response")

  detail="$(curl --fail --silent --show-error \
    "$api_url/api/v1/courses/$slug")"
  python3 -c '
import json, sys
payload = json.load(sys.stdin)
assert payload["course"]["status"] == "published", payload
assert payload["offers"], payload
' <<<"$detail"

  comparison="$(curl --fail --silent --show-error \
    "$api_url/api/v1/compare?offer_ids=$offer_id")"
  python3 -c '
import json, sys
payload = json.load(sys.stdin)
assert len(payload) == 1, payload
assert payload[0]["offer"]["id"], payload
' <<<"$comparison"
}

test -r "$catalog_file"
export CATALOG_OPERATOR="${CATALOG_OPERATOR:-compose-smoke}"

"${compose[@]}" up --detach --build
wait_for_ready
curl --fail --silent --show-error "$api_url/health/live" >/dev/null

"${compose[@]}" run --rm --no-deps --volume "$catalog_file:/data/catalog.json:ro" \
  api catalog validate /data/catalog.json
"${compose[@]}" run --rm --volume "$catalog_file:/data/catalog.json:ro" \
  api catalog import /data/catalog.json --dry-run
"${compose[@]}" run --rm --volume "$catalog_file:/data/catalog.json:ro" \
  api catalog import /data/catalog.json

assert_catalog

catalog_fingerprint() {
  "${compose[@]}" exec -T db psql -U "${POSTGRES_USER:-devcourse}" -d "${POSTGRES_DB:-devcourse}" -tAc \
    "SELECT md5(concat((SELECT jsonb_agg(to_jsonb(c) ORDER BY id) FROM courses c),(SELECT jsonb_agg(to_jsonb(o) ORDER BY id) FROM offers o),(SELECT jsonb_agg(to_jsonb(i) ORDER BY id) FROM imports i),(SELECT jsonb_agg(to_jsonb(s) ORDER BY key) FROM settings s)))"
}
before_restart="$(catalog_fingerprint)"

# `down` removes containers and the network but deliberately retains the named
# database volume. Starting the stack again must preserve the imported catalog.
"${compose[@]}" down
"${compose[@]}" up --detach --no-build
wait_for_ready
assert_catalog
[[ "$(catalog_fingerprint)" == "$before_restart" ]] || { echo "Restart changed persisted catalog/audit/settings" >&2; exit 1; }
for repetition in 1 2; do
  "${compose[@]}" run --rm --no-deps api migrate
  [[ "$(catalog_fingerprint)" == "$before_restart" ]] || { echo "Repeated migration changed persisted data" >&2; exit 1; }
done

echo "Compose smoke test passed"
