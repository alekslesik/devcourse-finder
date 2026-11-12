#!/usr/bin/env bash
# Run the unchanged browser suite with the exact locked Playwright browsers and
# system libraries. Avoid runtime APT dependency installation on hosted runners.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
version=$(node -p "require(process.argv[1]).packages['node_modules/@playwright/test'].version" "$root/frontend/package-lock.json")
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid locked Playwright version' >&2; exit 1; }
image="mcr.microsoft.com/playwright:v$version-noble"
docker pull "$image"
# The production stack and fixture servers use loopback on this same runner.
# Host networking lets the isolated browser reach those test-only endpoints.
docker run --rm --init --network host --ipc=host \
  --user "$(id -u):$(id -g)" \
  --mount "type=bind,src=$root,dst=/work" --workdir /work/frontend \
  -e CI -e E2E_REAL_API -e E2E_BROWSER -e npm_config_cache=/tmp/devcourse-npm-cache \
  "$image" npm run test:e2e -- "$@"
