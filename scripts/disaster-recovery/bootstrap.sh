#!/usr/bin/env bash
# Run on a clean Debian 13 server; then use restore.sh with a downloaded bundle.
set -Eeuo pipefail
[[ $EUID -eq 0 ]] || { echo 'Run as root' >&2; exit 1; }
source /etc/os-release
[[ $ID == debian && $VERSION_ID == 13 ]] || { echo 'Requires Debian 13; provision equivalent Docker/Compose/Python on other distributions.' >&2; exit 1; }
[[ $(uname -m) == x86_64 ]] || { echo 'Recovery images and pinned backup client require amd64.' >&2; exit 1; }
tools=$(cd -- "$(dirname -- "$0")" && pwd)
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y docker.io docker-compose python3 restic rclone ca-certificates curl
python3 "$tools/build-rclone.py" --ensure
install -m 0755 "$tools/bin/rclone" /usr/local/bin/rclone
systemctl enable --now docker
docker compose version
restic version
printf '%s\n' 'Ready. Configure /etc/devcourse-backup from your independent recovery kit, download a snapshot, then run restore.sh BUNDLE_DIRECTORY.'
