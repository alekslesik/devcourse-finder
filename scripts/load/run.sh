#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
report_dir=${LOAD_REPORT_DIR:-$root/.load-results}
mkdir -p "$report_dir"
report_dir=$(cd "$report_dir" && pwd)
if [[ ! "${LOAD_CPUSET:-0,1}" =~ ^[0-9]+,[0-9]+$ ]]; then
  echo "LOAD_CPUSET must contain exactly two CPU IDs" >&2; exit 1
fi
IFS=, read -r first_cpu second_cpu <<< "${LOAD_CPUSET:-0,1}"
[[ "$first_cpu" != "$second_cpu" ]] || { echo "Choose two distinct CPUs" >&2; exit 1; }
project="devcourse-load-$$"
compose=(docker compose -f "$root/scripts/load/compose.yaml" -p "$project")
cleanup() { "${compose[@]}" down --volumes --remove-orphans; }
trap cleanup EXIT
build_args=()
if [[ -n "${CODEX_PROXY_CERT:-}" ]]; then
  build_args+=(--secret "id=proxy_ca,src=$CODEX_PROXY_CERT")
fi
docker build "${build_args[@]}" -t devcourse-load-api "$root/backend"
python3 "$root/scripts/load/catalog.py" "$report_dir/catalog.json"
chmod 644 "$report_dir/catalog.json"
"${compose[@]}" up -d db
"${compose[@]}" run --rm api migrate
"${compose[@]}" run --rm -v "$report_dir/catalog.json:/data/catalog.json:ro" api catalog validate /data/catalog.json
"${compose[@]}" run --rm -v "$report_dir/catalog.json:/data/catalog.json:ro" api catalog import /data/catalog.json > "$report_dir/import.log"
"${compose[@]}" up -d api
api_id=$("${compose[@]}" ps -q api)
db_id=$("${compose[@]}" ps -q db)
docker inspect "$api_id" "$db_id" > "$report_dir/containers.json"
git -C "$root" rev-parse HEAD > "$report_dir/base-commit.txt"
for attempt in {1..60}; do
  if curl -fsS "http://127.0.0.1:${LOAD_PORT:-18080}/health/ready" > /dev/null; then break; fi
  if [[ "$attempt" == 60 ]]; then exit 1; fi
  sleep 1
done
"${compose[@]}" exec -T db psql -U devcourse -d devcourse_load_test -tAc \
  "SELECT count(*) FROM offers JOIN courses ON courses.id=offers.course_id WHERE status='published'" > "$report_dir/offer-count.txt"
[[ "$(cat "$report_dir/offer-count.txt")" == 10000 ]]
set +e
python3 "$root/scripts/load/measure.py" --url "http://127.0.0.1:${LOAD_PORT:-18080}" \
  --warmup "${LOAD_WARMUP:-60}" --duration "${LOAD_DURATION:-300}" --output "$report_dir/report.json"
result=$?
set -e
"${compose[@]}" logs --no-color api > "$report_dir/api.log"
docker stats --no-stream "$api_id" "$db_id" > "$report_dir/stats.txt"
echo "Report: $report_dir/report.json"
exit "$result"
