#!/usr/bin/env bash
# Run the unchanged browser suite with exact locked Playwright browsers/system
# libraries. Avoid runtime APT dependency installation on hosted runners.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
version=$(node -p "require(process.argv[1]).packages['node_modules/@playwright/test'].version" "$root/frontend/package-lock.json")
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid locked Playwright version' >&2; exit 1; }
image="mcr.microsoft.com/playwright:v$version-noble"
docker pull "$image"
controls=()
if [[ "${E2E_REAL_API:-}" == "1" ]]; then
  # Real-stack tests stop/restart their own API and inspect their own test DB.
  [[ ${COMPOSE_PROJECT_NAME:-} =~ ^devcourse-e2e-[0-9]+$ ]] || { echo 'Isolated real-test project is required' >&2; exit 1; }
  docker_cli=$(command -v docker)
  compose_plugin=$(docker info --format '{{range .ClientInfo.Plugins}}{{if eq .Name "compose"}}{{.Path}}{{end}}{{end}}')
  [[ -x $docker_cli && -x $compose_plugin && -S /var/run/docker.sock ]] || { echo 'Local Docker/Compose controls are required for real-stack tests' >&2; exit 1; }
  controls+=(--mount 'type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock'
    --mount "type=bind,src=$docker_cli,dst=/usr/local/bin/docker,readonly"
    --mount "type=bind,src=$compose_plugin,dst=/usr/local/lib/docker/cli-plugins/docker-compose,readonly"
    --group-add "$(stat -c %g /var/run/docker.sock)")
  for variable in COMPOSE_PROJECT_NAME E2E_API_IMAGE E2E_WEB_IMAGE POSTGRES_DB POSTGRES_USER POSTGRES_PASSWORD API_PORT WEB_PORT SITE_URL CATALOG_OPERATOR; do
    controls+=(-e "$variable")
  done
fi
# Keep the original absolute path so Compose resolves host bind mounts correctly.
# Host networking reaches the loopback-only app started by this test runner.
docker run --rm --init --network host --ipc=host \
  --user "$(id -u):$(id -g)" \
  --mount "type=bind,src=$root,dst=$root" --workdir "$root/frontend" \
  -e CI -e E2E_REAL_API -e E2E_BROWSER -e npm_config_cache=/tmp/devcourse-npm-cache \
  "${controls[@]}" "$image" npm run test:e2e -- "$@"
