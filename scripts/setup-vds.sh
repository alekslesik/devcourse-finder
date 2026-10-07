#!/usr/bin/env bash
# Prepare an Ubuntu host for DevCourse Finder's Docker Compose stack.
set -Eeuo pipefail

if (( EUID != 0 )); then
  echo 'Run this script with sudo bash setup-vds.sh.' >&2
  exit 1
fi

. /etc/os-release
if [[ "${ID:-}" != ubuntu || -z "${VERSION_CODENAME:-}" ]]; then
  echo 'This script requires Ubuntu with VERSION_CODENAME set.' >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl git openssl jq cron

# Keep an existing Docker installation and its containers and data.
if ! command -v docker >/dev/null 2>&1; then
  install -m 0755 -d /etc/apt/keyrings
  curl --fail --silent --show-error --location \
    https://download.docker.com/linux/ubuntu/gpg \
    --output /etc/apt/keyrings/devcourse-docker.asc
  chmod 0644 /etc/apt/keyrings/devcourse-docker.asc
  arch=$(dpkg --print-architecture)

  cat > /etc/apt/sources.list.d/devcourse-docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: ${VERSION_CODENAME}
Components: stable
Architectures: ${arch}
Signed-By: /etc/apt/keyrings/devcourse-docker.asc
EOF

  apt-get update
  apt-get install -y docker-ce docker-ce-cli containerd.io \
    docker-buildx-plugin docker-compose-plugin
fi

if ! compose_version=$(docker compose version --short); then
  echo 'Install Compose v2 from the package source used by your Docker installation, then rerun this script.' >&2
  exit 1
fi
if ! dpkg --compare-versions "${compose_version#v}" ge 2.24.0; then
  echo 'Docker Compose 2.24 or newer is required. Upgrade the plugin and rerun this script.' >&2
  exit 1
fi

systemctl enable --now cron
docker_unit_state=$(systemctl show --property=LoadState --value docker.service 2>/dev/null || true)
if [[ "$docker_unit_state" == loaded ]]; then
  systemctl enable --now docker.service
else
  echo 'No system docker.service found; preserving the existing Docker service manager.'
  echo 'Ensure that your Docker installation starts at boot using its own service manager.'
fi
docker info >/dev/null
mkdir -p /srv/devcourse-finder

echo 'DevCourse Finder host preparation complete.'
docker --version
docker compose version
echo 'Deployment directory: /srv/devcourse-finder'
echo 'Run subsequent deployment commands as root (sudo -i, then cd /srv/devcourse-finder).'
echo 'Next: check out the repository, configure production .env and HTTPS, and start Compose.'
