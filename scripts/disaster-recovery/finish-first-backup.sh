#!/usr/bin/env bash
# Resume a verified initial staging bundle, then enable scheduling only after a remote drill.
set -Eeuo pipefail
[[ $EUID -eq 0 ]] || { echo 'Run as root' >&2; exit 1; }
tools=$(cd -- "$(dirname -- "$0")" && pwd)
stage=/var/lib/devcourse-backup/bundle
if [[ -f "$stage/manifest.json" ]]; then
    python3 "$tools/recovery.py" upload "$stage"
else
    bash "$tools/backup.sh"
fi
snapshot=$(python3 - <<'PY'
import json
from pathlib import Path
print(json.loads(Path('/var/lib/devcourse-backup/last-success.json').read_text())['snapshot'])
PY
)
python3 "$tools/recovery.py" drill "$snapshot"
bash "$tools/install-timers.sh"
python3 - <<'PY'
import shutil
from pathlib import Path
stage=Path('/var/lib/devcourse-backup/bundle')
if stage.is_symlink(): raise SystemExit('Refusing unexpected staging symlink')
if stage.exists(): shutil.rmtree(stage)
PY
printf 'Initial external backup and recovery drill completed: %s\n' "$snapshot"
