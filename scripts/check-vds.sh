#!/usr/bin/env bash
# Read-only deployment checks. --https also checks the public HTTPS endpoint.
set -uo pipefail

if [[ $# -gt 1 || (${1:-} != '' && ${1:-} != --https) ]]; then
  echo 'Usage: sudo bash check-vds.sh [--https]' >&2
  exit 2
fi
[[ $EUID -eq 0 ]] || { echo 'Run: sudo bash check-vds.sh' >&2; exit 2; }
for command in docker jq curl timeout stat; do
  command -v "$command" >/dev/null || {
    echo "[FAIL] Missing command: $command" >&2; exit 2;
  }
done

failures=0
pass() { printf '[PASS] %s\n' "$1"; }
fail() { printf '[FAIL] %s\n' "$1"; failures=$((failures + 1)); }
if ! cd /srv/devcourse-finder; then
  echo '[FAIL] Deployment directory is missing.' >&2
  exit 1
fi
compose=(docker compose -f compose.yaml -f compose.production.yaml)
if ! timeout --kill-after=2s 20 docker info >/dev/null 2>&1; then
  echo '[FAIL] Docker daemon is unavailable to this root session.' >&2
  exit 1
fi
pass 'Docker daemon is accessible'

if [[ -f .env && ! -L .env ]]; then
  permissions=$(stat -c '%a' .env)
  if [[ $permissions =~ ^[0-7]{3,4}$ ]] && (( (8#$permissions & 077) == 0 )); then
    pass '.env has no group or other permissions'
  else
    fail '.env permissions: run chmod 600 /srv/devcourse-finder/.env'
  fi
else
  fail 'A regular .env file is required'
fi
if ! config=$(timeout --kill-after=2s 20 "${compose[@]}" config --format json 2>/dev/null); then
  echo '[FAIL] Compose configuration is invalid; run docker compose -f compose.yaml -f compose.production.yaml config --quiet.' >&2
  exit 1
fi
if jq -e '
  (.services.frontend.ports | length == 1) and
  (.services.frontend.ports[0].host_ip == "127.0.0.1") and
  (.services.frontend.ports[0].published | tostring == "3000") and
  ((.services.api.ports // []) | length == 0) and
  ((.services.db.ports // []) | length == 0) and
  (.services.frontend.environment.SITE_URL // "" |
    test("^https://[a-zA-Z0-9.-]+(:[0-9]+)?$"))
' <<<"$config" >/dev/null 2>&1; then
  pass 'Production configuration: private ports and HTTPS site origin'
else
  fail 'Configuration requires WEB_PORT=127.0.0.1:3000, private API/database ports, and an HTTPS SITE_URL'
fi
site_url=$(jq -r '.services.frontend.environment.SITE_URL // ""' <<<"$config")
services=(db migrate api frontend)
if jq -e '.services | has("catalog-updater")' <<<"$config" >/dev/null; then
  services+=(catalog-updater)
fi
unset config

for service in "${services[@]}"; do
  if ! container=$(timeout --kill-after=2s 20 "${compose[@]}" ps --all --quiet "$service" 2>/dev/null) || [[ -z $container || $container == *$'\n'* ]]; then
    fail "$service: expected exactly one existing container"
    continue
  fi
  if ! state=$(timeout --kill-after=2s 20 docker inspect --format '{{json .State}}' "$container" 2>/dev/null); then
    fail "$service: cannot inspect container state"
    continue
  fi
  if [[ $service == migrate ]]; then
    if jq -e '.Status == "exited" and .ExitCode == 0' <<<"$state" >/dev/null; then
      pass 'migrate: completed successfully (exit 0)'
    else
      fail 'migrate: has not completed successfully'
    fi
    continue
  fi
  if jq -e '.Status == "running" and ((.Health.Status // "healthy") == "healthy")' <<<"$state" >/dev/null; then
    pass "$service: running; health check passed if configured"
  else
    fail "$service: not running or health check has not passed"
  fi
  # Check actual containers as well as the desired Compose configuration.
  if ! ports=$(timeout --kill-after=2s 20 docker inspect --format '{{json .NetworkSettings.Ports}}' "$container" 2>/dev/null); then
    fail "$service: cannot inspect published ports"
  elif [[ $service == frontend ]]; then
    if jq -e '
      (."3000/tcp" | length == 1) and
      (."3000/tcp"[0].HostIp == "127.0.0.1") and
      (."3000/tcp"[0].HostPort == "3000") and
      (to_entries | all(.key == "3000/tcp" or ((.value // []) | length == 0)))
    ' <<<"$ports" >/dev/null; then
      pass 'frontend: actual port binding is 127.0.0.1:3000'
    else
      fail 'frontend: actual port bindings differ from the private deployment configuration'
    fi
  elif jq -e '(. // {}) | to_entries | all((.value // []) | length == 0)' <<<"$ports" >/dev/null; then
    pass "$service: no published host ports"
  else
    fail "$service: unexpected published host ports"
  fi
  if [[ $service == db ]]; then
    if timeout --kill-after=2s 20 docker inspect --format '{{json .Mounts}}' "$container" 2>/dev/null |
      jq -e 'any(.[]; .Type == "volume" and .Destination == "/var/lib/postgresql/data")' >/dev/null; then
      pass 'db: persistent data volume is mounted'
    else
      fail 'db: persistent data volume is missing'
    fi
  fi
done

echo '[CHECK] PostgreSQL SQL query (connection/query limits: 5 seconds; outer limit: 15 seconds)'
if timeout --kill-after=2s 15 "${compose[@]}" exec --interactive=false -T db sh -c \
  'PGCONNECT_TIMEOUT=5 PGOPTIONS="-c statement_timeout=5000" psql --no-psqlrc --no-password --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --tuples-only --no-align --command "SELECT 1"' </dev/null 2>/dev/null |
  jq -e '. == 1' >/dev/null 2>&1; then
  pass 'PostgreSQL: read-only SQL query succeeded'
else
  fail 'PostgreSQL: SQL query failed'
fi
echo '[CHECK] API readiness (limit: 15 seconds)'
if timeout --kill-after=2s 15 "${compose[@]}" exec --interactive=false -T api wget -Y off -T 10 -qO- \
  http://127.0.0.1:8080/health/ready </dev/null 2>/dev/null |
  jq -e '.ok == true' >/dev/null 2>&1; then
  pass 'API readiness: database connection is available'
else
  fail 'API readiness check failed'
fi
echo '[CHECK] Frontend home page (limit: 15 seconds)'
if [[ $(curl --silent --output /dev/null --write-out '%{http_code}' --connect-timeout 5 --max-time 15 \
  http://127.0.0.1:3000/) == 200 ]]; then
  pass 'Frontend home page: HTTP 200'
else
  fail 'Frontend home page did not return HTTP 200'
fi
echo '[CHECK] Catalog through frontend (limit: 15 seconds)'
if catalog=$(curl --fail --silent --connect-timeout 5 --max-time 15 http://127.0.0.1:3000/api/v1/courses) &&
  jq -e '(.items | type == "array") and (.total | type == "number" and . >= 0)' <<<"$catalog" >/dev/null 2>&1; then
  total=$(jq -r '.total' <<<"$catalog")
  pass "Frontend → API → database: catalog response is valid ($total courses)"
  if [[ $total == 0 ]]; then
    echo '[INFO] An empty catalog is normal until a verified import or automatic collection completes.'
  fi
else
  fail 'Catalog request through the frontend failed or returned invalid JSON'
fi

if [[ ${1:-} == --https ]]; then
  echo '[CHECK] Public HTTPS home page and catalog (limit: 20 seconds per request)'
  if [[ $site_url =~ ^https://[a-zA-Z0-9.-]+(:[0-9]+)?$ ]] &&
    [[ $(curl --silent --location --proto '=https' --proto-redir '=https' \
      --output /dev/null --write-out '%{http_code}' --max-time 20 "$site_url/") == 200 ]] &&
    curl --fail --silent --location --proto '=https' --proto-redir '=https' --max-time 20 \
      "$site_url/api/v1/courses" | jq -e '(.items | type == "array") and (.total | type == "number")' >/dev/null 2>&1; then
    pass 'Public HTTPS: trusted TLS, home page, and catalog API are accessible from this VDS'
  else
    fail 'Public HTTPS check failed: inspect DNS, reverse proxy, and certificate'
  fi
else
  echo '[INFO] Public HTTPS was not checked. Run with --https after reverse proxy setup.'
fi

echo
if (( failures == 0 )); then
  echo 'RESULT: PASS — local deployment checks passed.'
  echo 'Backup scheduling and catalog collection results must be verified separately.'
else
  echo "RESULT: FAIL — $failures check(s) failed."
  echo 'Inspect: docker compose -f compose.yaml -f compose.production.yaml logs --tail=100'
  exit 1
fi
