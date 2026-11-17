#!/usr/bin/env bash
# Install only after the initial backup and isolated restore have succeeded.
set -Eeuo pipefail
[[ $EUID -eq 0 ]] || { echo 'Run as root' >&2; exit 1; }
python3 /opt/devcourse-backup/recovery.py status
[[ -s /var/lib/devcourse-backup/last-drill.json ]]
cat > /etc/systemd/system/devcourse-full-backup.service <<'UNIT'
[Unit]
Description=Encrypted full DevCourseFinder backup to Yandex Disk
Requires=docker.service
After=docker.service network-online.target
OnFailure=devcourse-backup-alert.service
Wants=network-online.target
[Service]
Type=oneshot
ExecStart=/opt/devcourse-backup/backup.sh
UMask=0077
Nice=10
IOSchedulingClass=best-effort
IOSchedulingPriority=7
TimeoutStartSec=3h
MemoryMax=700M
UNIT
cat > /etc/systemd/system/devcourse-full-backup.timer <<'UNIT'
[Unit]
Description=Daily full backup at 03:00 Moscow
[Timer]
OnCalendar=*-*-* 03:00:00 Europe/Moscow
Persistent=true
[Install]
WantedBy=timers.target
UNIT
cat > /etc/systemd/system/devcourse-backup-monitor.service <<'UNIT'
[Unit]
Description=Check that the last external backup is successful and younger than 30 hours
OnFailure=devcourse-backup-alert.service
[Service]
Type=oneshot
ExecStart=/usr/bin/python3 /opt/devcourse-backup/recovery.py status
UMask=0077
UNIT
cat > /etc/systemd/system/devcourse-backup-monitor.timer <<'UNIT'
[Unit]
Description=Hourly backup freshness check
[Timer]
OnBootSec=15min
OnUnitActiveSec=1h
[Install]
WantedBy=timers.target
UNIT
cat > /etc/systemd/system/devcourse-backup-drill.service <<'UNIT'
[Unit]
Description=Monthly isolated recovery from Yandex Disk
Requires=docker.service
After=docker.service network-online.target
OnFailure=devcourse-backup-alert.service
[Service]
Type=oneshot
ExecStart=/usr/bin/python3 /opt/devcourse-backup/recovery.py drill latest
UMask=0077
Nice=10
TimeoutStartSec=3h
MemoryMax=700M
UNIT
cat > /etc/systemd/system/devcourse-backup-drill.timer <<'UNIT'
[Unit]
Description=Monthly recovery drill
[Timer]
OnCalendar=*-*-01 05:00:00 Europe/Moscow
Persistent=true
[Install]
WantedBy=timers.target
UNIT
cat > /etc/systemd/system/devcourse-backup-prune.service <<'UNIT'
[Unit]
Description=Weekly removal of unreferenced encrypted backup chunks
After=network-online.target
OnFailure=devcourse-backup-alert.service
[Service]
Type=oneshot
ExecStart=/usr/bin/python3 /opt/devcourse-backup/recovery.py prune
UMask=0077
Nice=10
TimeoutStartSec=3h
MemoryMax=700M
UNIT
cat > /etc/systemd/system/devcourse-backup-prune.timer <<'UNIT'
[Unit]
Description=Weekly encrypted repository cleanup
[Timer]
OnCalendar=Sun *-*-* 04:00:00 Europe/Moscow
Persistent=true
[Install]
WantedBy=timers.target
UNIT
cat > /etc/systemd/system/devcourse-backup-alert.service <<'UNIT'
[Unit]
Description=Send backup failure email through configured SMTP
ConditionPathExists=/etc/devcourse-backup/email.json
[Service]
Type=oneshot
ExecStart=/usr/bin/python3 /opt/devcourse-backup/notify.py failure
UMask=0077
UNIT
systemctl daemon-reload
systemctl enable --now devcourse-full-backup.timer devcourse-backup-monitor.timer devcourse-backup-drill.timer devcourse-backup-prune.timer
# Keep the previous local database backup schedule as an additional safety net.
systemctl start devcourse-backup-monitor.service
systemctl list-timers --all 'devcourse-*' --no-pager
