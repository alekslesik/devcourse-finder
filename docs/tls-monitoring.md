# TLS monitoring for devcoursefinder.ru

The current VDS serves the purchased GlobalSign AlphaSSL certificate through
Caddy. Its verified expiry is 25 May 2027 at 16:34:52 Europe/Moscow. The
monitor does not replace that certificate or claim that the issuer renewed it.

The `TLS certificate monitoring` workflow checks the real public certificate
every day at 09:20 Europe/Moscow and supports a manual dispatch. It verifies the
certificate chain and hostname using the runner's normal trusted CA store. It
warns at 45 days and becomes critical at 14 days. TLS connection, expiry and trust
failures also fail the check.

The workflow uses its repository-scoped Actions token to maintain a marked bot
issue. Repeated warnings do not create duplicate issues or daily comments.
Severity changes update that issue. A successful check with more than 45 days
remaining closes only the monitor's own bot issue. It never closes the owner's
renewal task or unrelated issues. Each run saves the JSON report and Actions
summary, including the actual expiry and SHA256 certificate fingerprint. A
notification API failure fails the workflow rather than pretending an alert was
delivered. GitHub notifications depend on the recipient's repository subscription
settings; no email or messaging integration is configured by this workflow.

Manual public check from an ordinary external machine:

```sh
python3 scripts/check-tls-expiry.py --host devcoursefinder.ru
python3 scripts/check-public-https.py --host devcoursefinder.ru
```

The expiry check uses direct TLS so it measures the origin certificate, not the
certificate of an intercepting HTTP proxy. In the managed Cloud environment,
run it on the authorized VDS over the verified SSH connection. On the VDS it can
connect locally to Caddy while still validating the public hostname and chain:

```sh
python3 scripts/check-tls-expiry.py --host devcoursefinder.ru --connect-address 127.0.0.1
```

Do not run the historical Ubuntu/Nginx `setup-https.sh` on the current
Debian/Caddy server. Its deployment and pinned-host workflows target the former
server and require a separate reviewed migration. The live Caddy configuration,
paid certificate/key and `.env` remain outside the Git releases.

After obtaining the renewed leaf and issuing chain from GlobalSign/FirstVDS,
stage the concatenated full chain and matching key in a root-only directory.
Use the released script on the VDS (validation is the default):

```sh
chmod 600 /root/tls-renewal/privkey.key
python3 /srv/devcourse-finder/current/scripts/rotate-paid-tls.py --fullchain /root/tls-renewal/fullchain.pem --key /root/tls-renewal/privkey.key
# Install the same validated pair, retain a protected backup and hot-reload Caddy:
python3 /srv/devcourse-finder/current/scripts/rotate-paid-tls.py --fullchain /root/tls-renewal/fullchain.pem --key /root/tls-renewal/privkey.key --apply
python3 /srv/devcourse-finder/current/scripts/check-public-https.py
```

The helper verifies the hostname and trusted intermediate chain, the retained
GlobalSign issuer, validity (at least 30 days), matching public keys and extended
expiry. It shares the deployment lock. Installation keeps a root-only rollback
pair, validates and force-reloads Caddy (so an unchanged Caddyfile reloads the supplied
certificate files), then checks the actual locally served trusted
certificate fingerprint. Reload or fingerprint failure restores and reloads the
old pair; a failed rollback is an explicit failure. An already installed pair is
checked without rewriting files or reloading. Default validation never replaces
the running certificate. No private key is printed or included in reports.

Issue #186 remains open for the actual issuer renewal and live new-pair
installation. Monitoring and a rotation helper cannot renew a paid certificate
without the issuer's delivery. The current server runbook is
`/srv/devcourse-finder/ingress/TLS-OPERATIONS.md`.
