# Full encrypted backups and recovery

These scripts target the deployed Debian 13 host, Docker Compose application
`devcourse-finder`, PostgreSQL 17, and the separate Caddy project
`devcourse-ingress`. Install a copy in `/opt/devcourse-backup` (root-only).
They never read credentials from Git. All working files are mode 0700/0600.

## Storage and credentials

`/etc/devcourse-backup/config.json` contains:

```json
{"repository":"rclone:yandex:backups/devcourse-finder"}
```

`/etc/devcourse-backup/rclone.conf` is a root-only rclone WebDAV configuration:
remote `yandex`, vendor `yandex`, URL `https://webdav.yandex.ru`, username and
an rclone-obscured app password. Obscuring is NOT encryption; protect this file.
`/etc/devcourse-backup/password` contains a separate randomly generated restic
password. Keep the independent recovery kit in a password manager, alongside
Yandex access and this runbook. Never upload the recovery password into its own
repository or commit it. Losing all independent copies prevents recovery.

For direct restic commands (root shell):

```bash
export RCLONE_CONFIG=/etc/devcourse-backup/rclone.conf
export RESTIC_PASSWORD_FILE=/etc/devcourse-backup/password
export RESTIC_REPOSITORY=rclone:yandex:backups/devcourse-finder
restic snapshots --tag devcourse-full-v1
```

## Daily backup

`backup.sh` acquires the same deployment lock as release/catalog operations.
It makes an MVCC-consistent custom-format `pg_dump` (not a live volume copy),
saves PostgreSQL globals for disaster investigation, the deployed `.env`, current
release and source directories of existing containers, original Compose files,
Dockerfiles, read-only data binds, ingress configuration/TLS, and Caddy volumes.
It saves every actual container image by content ID with `docker image save`,
including stopped migration containers. This handles containers that were reused
from older releases. Saved resolved Compose configurations use those same IDs,
actual container environment and `pull_policy: never`. No registry, mutable tag,
build tooling, or source repository availability is required for recovery.

The manifest records UTC time, deployed version/commit, PostgreSQL version,
image IDs, active services, volume names, consistency rules and SHA-256 hashes.
Secrets and resolved configurations enter only the encrypted repository; staging
is removed on completion/failure. No production service is stopped for backup.
Application binds must be read-only and under the deployment directory; new
writable binds or application volumes fail closed pending an explicit consistency
plan. Caddy volumes contain proxy state, not transactional application files;
they are copied separately. Its currently used certificate/key is in the captured
read-only TLS bind. PostgreSQL globals are captured separately from the database
transaction. This is a single application database backup, not cluster PITR.

After success, retention keeps 7 daily, 4 weekly and 6 monthly snapshots selected
from daily backups; overlapping selections share one snapshot. Only this backup's
tag, host and staging path are eligible. `forget` removes expired snapshot
references; a Sunday 04:00 Moscow `prune` timer reclaims unreferenced encrypted
chunks. You can also run `recovery.py prune` during a maintenance window. It does not affect old local database dumps.
A failed upload never applies retention. Keep enough disk space (approximately
4 GB for staging and at least another 4 GB for a recovery drill, growing with DB).

## Scheduling and monitoring

After the first backup AND drill pass, run `install-timers.sh`. Daily full backup:
03:00 Europe/Moscow; monthly drill: day 1 at 05:00 Europe/Moscow. The previous local
03:15 database-dump timer is retained as a separate safety net. `Persistent=true`
catches missed runs. CPU/IO priority and process memory are bounded. Docker
container memory is separate; the drill database is limited to 512 MiB.

```bash
/opt/devcourse-backup/backup.sh
python3 /opt/devcourse-backup/recovery.py status
python3 /opt/devcourse-backup/recovery.py check
journalctl -u devcourse-full-backup.service
systemctl list-timers 'devcourse-*'
```

`/var/lib/devcourse-backup/last-success.json` stores the last successful external
snapshot. A failed backup writes `last-failure.json`; success clears it. Hourly
monitoring exits nonzero after failure or if success is older than 30 hours.
Systemd records failures, including process kills/timeouts. Failure hooks call
`notify.py failure` when `/etc/devcourse-backup/email.json` exists. Configure SMTP
fields `host`, `port`, `ssl`, `user`, `password`, `from`, `to` in that root-only
file, then run `python3 /opt/devcourse-backup/notify.py test`. TLS verification
is mandatory (implicit TLS or STARTTLS). Repeated alerts are limited to one per
12 hours; a successful backup sends recovery mail after an earlier alert. Check
receipt of the test email before declaring notification delivery configured.
For Yandex Mail, run `python3 /opt/devcourse-backup/configure-email.py --to RECIPIENT_EMAIL`
in an interactive root console. It asks for the sender address and a hidden
**Mail-scoped** Yandex application password, tests SMTP with verified TLS at
`smtp.yandex.ru:465`, and preserves the prior config if the test fails. A WebDAV
application password is not a replacement for a Mail-scoped password. Automated
setup can inherit `BACKUP_SMTP_USER` and `BACKUP_SMTP_PASSWORD` from managed
secrets, without putting their values in command arguments. Recovery verification is recorded separately
in `last-drill.json`. The monitor checks backup freshness, not full server outage;
independent off-host monitoring is still necessary for a total VDS failure.

## Isolated recovery test

```bash
python3 /opt/devcourse-backup/recovery.py drill SNAPSHOT_ID
```

Downloads the encrypted remote snapshot, checks every file hash, loads the exact
images, and restores into a unique Compose project, internal network and fresh
PostgreSQL volume. No host ports, workers, migration service or Caddy are started.
The restored stack cannot contact the internet. Verifies SQL access/course row
count, API readiness, frontend HTML and its real catalog API proxy, then deletes
only its own containers/network/volume and downloaded bundle. Production is
unmodified. Initial and scheduled drills validate DB/application recovery on the
existing Docker host; a clean-machine public DNS/HTTPS cutover must be validated
separately before declaring a whole-server replacement complete.

## Recover onto a clean server

1. Provision Debian 13 on the same architecture as the saved images (amd64).
   Copy these scripts from the independent runbook/source or recovery kit.
2. Run `bootstrap.sh` to install Docker/Compose, Python, restic and rclone.
3. Recreate `/etc/devcourse-backup` from the independent recovery kit, mode 0700
   for the directory and 0600 for its files. Set the restic environment above.
4. Choose an explicit snapshot ID using `restic snapshots`. Download it:

   ```bash
   mkdir -m 700 /var/tmp/devcourse-recovery
   restic restore SNAPSHOT_ID --target /var/tmp/devcourse-recovery
   ```

5. Restore with:

   ```bash
   /opt/devcourse-backup/restore.sh /var/tmp/devcourse-recovery/var/lib/devcourse-backup/bundle
   ```

   Refuses an existing deployment or containers. It verifies checksums, restores
   files/Caddy volumes, loads exact images, starts the compatible PostgreSQL
   image, restores the DB, then starts the captured running application services
   and Caddy. It does not run migrations or upgrades. Resolved pinned Compose
   files are kept in `/srv/devcourse-finder/recovery` for subsequent verification.
   The standard application user/password is recreated by the saved `.env` via
   the database container's environment. `globals.sql` preserves other role
   definitions for explicit DBA review; extra cluster roles/tablespaces require
   review before restore (the deployed app uses its single DB owner).
6. Verify `https://devcoursefinder.ru/`, `/api/v1/courses`, displayed version,
   paid TLS certificate expiry, external DNS and ports 80/443. Reconfigure SSH,
   firewall and Tailscale for the new host as appropriate. Those are host access
   controls, not application data. Keep original server isolated until cutover.
7. Run a new backup and drill, then install timers. Remove downloaded plaintext
   recovery files after verification. Perform future upgrades/migrations as a
   separate operation after recovery succeeds.

## Replace only an existing application database

```bash
/opt/devcourse-backup/restore-db.sh /root/chosen/database.dump --confirm 'REPLACE devcourse-finder'
```

This requires an explicit confirmation string, the deployment lock and the
backup-operation lock. It stops app writers, makes a fresh safety dump under
`/srv/devcourse-finder/backups/before-restore-*.dump`, then drops/recreates and
restores the application DB with `pg_restore --exit-on-error`. A failure leaves
writers stopped and preserves the safety dump. Recover the safety dump or fix
the issue before restarting services. Choose a compatible application version
first; no automatic upgrades or migrations run. `verify.sh BUNDLE_DIRECTORY`
checks the stack restored using the saved pinned Compose file.
