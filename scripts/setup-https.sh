#!/usr/bin/env bash
# Configure Nginx and Let's Encrypt after deploy-vds.sh succeeds.
set -Eeuo pipefail
trap 'printf "HTTPS setup failed at line %s. Fix the reported error and rerun.\n" "$LINENO" >&2' ERR

[[ $EUID -eq 0 ]] || { echo 'Run: sudo bash setup-https.sh' >&2; exit 1; }
. /etc/os-release
[[ ${ID:-} == ubuntu ]] || { echo 'This script requires Ubuntu.' >&2; exit 1; }
domain=${DOMAIN:-alekslesik.fvds.ru}
public_ip=${PUBLIC_IP:-83.220.174.166}
email=${LE_EMAIL:-}
[[ $domain =~ ^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$ ]] || {
  echo 'DOMAIN must be a lowercase DNS hostname.' >&2; exit 1;
}
for command in docker jq curl getent; do
  command -v "$command" >/dev/null || {
    echo "Missing $command. Run setup-vds.sh first." >&2; exit 1;
  }
done
cd /srv/devcourse-finder
compose=(docker compose -f compose.yaml -f compose.production.yaml)
if ! "${compose[@]}" config --format json |
  jq -e --arg origin "https://$domain" '
    .services.frontend.environment.SITE_URL == $origin and
    (.services.frontend.ports | length == 1) and
    .services.frontend.ports[0].host_ip == "127.0.0.1" and
    (.services.frontend.ports[0].published | tostring == "3000")
  ' >/dev/null; then
  echo "Set SITE_URL=https://$domain and WEB_PORT=127.0.0.1:3000 in .env, then rerun deploy-vds.sh." >&2
  exit 1
fi
curl --fail --silent --show-error --noproxy '*' --connect-timeout 5 --max-time 15 \
  http://127.0.0.1:3000/ >/dev/null
addresses=$(getent ahostsv4 "$domain" | awk '{print $1}' | sort -u)
[[ $addresses == "$public_ip" ]] || {
  echo "IPv4 DNS for $domain must point only to this VDS ($public_ip). Check DNS before retrying." >&2
  exit 1
}

echo '[STEP] Installing Nginx and Certbot'
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y nginx certbot python3 python3-certbot-nginx
site=/etc/nginx/sites-available/devcourse-finder
enabled=/etc/nginx/sites-enabled/devcourse-finder
marker="# Managed by DevCourse Finder setup-https.sh for $domain"
new_site=false
new_link=false
if [[ -e $site || -L $site ]]; then
  [[ -f $site && ! -L $site ]] && grep -Fxq "$marker" "$site" || {
    echo "Refusing to replace an unmanaged Nginx configuration: $site" >&2; exit 1;
  }
fi
# Inspect all active includes, not just sites-enabled or nginx -t's exit code.
# Duplicate server names are warnings and can otherwise shadow this proxy.
python3 - "$domain" "$site" <<'PY'
import os
import re
import shlex
import subprocess
import sys

domain, managed = sys.argv[1:]
result = subprocess.run(["nginx", "-T"], capture_output=True, text=True)
if result.returncode:
    raise SystemExit("Cannot inspect active Nginx configuration; run nginx -t and fix its errors.")
sections = re.split(r"^# configuration file (.+):\s*$", result.stdout, flags=re.MULTILINE)
if len(sections) < 3:
    raise SystemExit("Nginx did not return its active configuration; refusing to enable a site.")
for path, body in zip(sections[1::2], sections[2::2]):
    if os.path.realpath(path) == os.path.realpath(managed):
        continue
    lexer = shlex.shlex(body, posix=True, punctuation_chars=";{}")
    lexer.whitespace_split = True
    tokens = iter(lexer)
    for token in tokens:
        if token != "server_name":
            continue
        for name in tokens:
            if ";" in name:
                break
            normalized = name.lower()
            matches = normalized == domain
            if normalized.startswith("*."):
                matches = domain.endswith(normalized[1:])
            elif normalized.startswith("."):
                matches = domain == normalized[1:] or domain.endswith(normalized)
            elif normalized.endswith(".*"):
                matches = domain.startswith(normalized[:-1])
            elif name.startswith("~"):
                # Nginx uses PCRE, which is not Python's regex syntax. Do not
                # guess whether an arbitrary regex virtual host can match.
                raise SystemExit(f"A regex server name in {path} needs manual conflict review before enabling this domain.")
            if matches:
                raise SystemExit(f"Domain {domain} is already handled by another active Nginx configuration: {path}. Resolve the conflict before retrying.")
print("[PASS] No domain conflict in other active Nginx configuration files")
PY
if [[ ! -e $site ]]; then
  cat > "$site" <<EOF
$marker
server {
    listen 80;
    listen [::]:80;
    server_name $domain;
    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 60s;
    }
}
EOF
  new_site=true
fi
if [[ -e $enabled || -L $enabled ]]; then
  [[ -L $enabled && $(readlink -f "$enabled") == "$site" ]] || {
    if $new_site; then rm -f "$site"; fi
    echo "Refusing to replace an existing Nginx site entry: $enabled" >&2; exit 1;
  }
else
  ln -s "$site" "$enabled"
  new_link=true
fi
if ! nginx -t; then
  if $new_link; then rm -f "$enabled"; fi
  if $new_site; then rm -f "$site"; fi
  echo 'Nginx validation failed; newly created site files were removed.' >&2
  exit 1
fi
systemctl enable --now nginx
systemctl reload nginx
if command -v ufw >/dev/null && ufw status | grep -q '^Status: active'; then
  ufw allow 80/tcp
  ufw allow 443/tcp
fi

echo '[STEP] Issuing/installing certificate; public ports 80 and 443 must be reachable'
cert_name="devcourse-finder-$domain"
contact=(--register-unsafely-without-email)
if [[ -n $email ]]; then contact=(--email "$email"); fi
certbot --nginx --non-interactive --agree-tos --redirect --keep-until-expiring \
  --cert-name "$cert_name" -d "$domain" "${contact[@]}"
nginx -t
systemctl reload nginx
systemctl enable --now certbot.timer
echo '[STEP] Testing renewal against the staging CA (may take several minutes)'
certbot renew --cert-name "$cert_name" --dry-run

echo '[STEP] Checking HTTPS'
[[ $(curl --silent --show-error --location --proto '=https' --proto-redir '=https' \
  --output /dev/null --write-out '%{http_code}' --connect-timeout 5 --max-time 20 \
  "https://$domain/") == 200 ]] || { echo 'HTTPS home page check failed.' >&2; exit 1; }
curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
  --connect-timeout 5 --max-time 20 "https://$domain/api/v1/courses" |
  jq -e '(.items | type == "array") and (.total | type == "number")' >/dev/null
echo "HTTPS setup complete: https://$domain"
echo 'Automatic certificate renewal is enabled and its dry run passed.'
if [[ -f /root/check-vds.sh ]]; then
  echo 'Verify deployment: bash /root/check-vds.sh --https'
elif [[ -f /srv/devcourse-finder/scripts/check-vds.sh ]]; then
  echo 'Verify deployment: bash /srv/devcourse-finder/scripts/check-vds.sh --https'
else
  echo "Verify HTTPS: curl --fail --show-error https://$domain/api/v1/courses"
fi
echo 'Next: configure database backups and publish the catalog.'
