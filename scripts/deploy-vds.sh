#!/usr/bin/env bash
# Check out and start DevCourse Finder after setup-vds.sh.
set -Eeuo pipefail

[[ $EUID -eq 0 ]] || { echo 'Run: sudo bash deploy-vds.sh' >&2; exit 1; }
for command in git docker openssl curl jq; do
  command -v "$command" >/dev/null || {
    echo "Missing $command. Run setup-vds.sh first." >&2; exit 1;
  }
done
version=$(docker compose version --short)
dpkg --compare-versions "${version#v}" ge 2.24.0 || {
  echo 'Docker Compose 2.24 or newer is required.' >&2; exit 1;
}
docker info >/dev/null

app_dir=/srv/devcourse-finder
repo=https://github.com/alekslesik/devcourse-finder.git
site_url=${SITE_URL:-https://alekslesik.fvds.ru}

if [[ ! -d "$app_dir/.git" ]]; then
  git clone "$repo" "$app_dir"
fi
cd "$app_dir"
[[ $(git remote get-url origin) == "$repo" ]] || {
  echo 'The deployment directory belongs to a different repository.' >&2; exit 1;
}
for file in compose.yaml compose.production.yaml; do
  [[ -f $file ]] || { echo "Missing $file." >&2; exit 1; }
done

[[ ! -L .env ]] || { echo 'Refusing to use a symlink for .env.' >&2; exit 1; }
if [[ ! -e .env ]]; then
  [[ $site_url =~ ^https://[a-zA-Z0-9.-]+(:[0-9]+)?$ ]] || {
    echo 'SITE_URL must be an HTTPS origin without a trailing slash.' >&2; exit 1;
  }
  if docker volume inspect devcourse-finder_postgres-data >/dev/null 2>&1; then
    echo 'An existing database volume was found. Restore its original .env before proceeding.' >&2
    exit 1
  fi
  (
    umask 077
    set -o noclobber
    cat > .env <<EOF
POSTGRES_DB=devcourse
POSTGRES_USER=devcourse
POSTGRES_PASSWORD=$(openssl rand -hex 32)
CATALOG_OPERATOR=vds-admin
WEB_PORT=127.0.0.1:3000
SITE_URL=$site_url
EOF
  )
  echo 'Created production .env with a random database password.'
else
  echo 'Keeping the existing .env and database password.'
fi
chmod 600 .env

compose=(docker compose -f compose.yaml -f compose.production.yaml)
# Validate configuration without printing database credentials.
"${compose[@]}" config --format json | jq -e '
  (.services.frontend.ports | length == 1) and
  (.services.frontend.ports[0].host_ip == "127.0.0.1") and
  (.services.frontend.ports[0].published | tostring == "3000") and
  ((.services.api.ports // []) | length == 0) and
  ((.services.db.ports // []) | length == 0) and
  (.services.frontend.environment.SITE_URL // "" |
    test("^https://[a-zA-Z0-9.-]+(:[0-9]+)?$"))
' >/dev/null || {
  echo 'Configuration must keep API/database ports private and set WEB_PORT=127.0.0.1:3000.' >&2
  echo 'The effective frontend SITE_URL must be an HTTPS origin without a trailing slash; check .env and exported variables.' >&2
  exit 1
}

"${compose[@]}" up --build -d
"${compose[@]}" ps
for attempt in {1..60}; do
  if curl --fail --silent --max-time 5 \
    http://127.0.0.1:3000/api/v1/courses >/dev/null; then
    echo 'Application is responding at http://127.0.0.1:3000.'
    echo 'Next: configure an HTTPS reverse proxy, daily backups, and catalog publication.'
    exit 0
  fi
  sleep 2
done
echo 'Application check failed. Inspect: docker compose -f compose.yaml -f compose.production.yaml logs --tail=100' >&2
exit 1
