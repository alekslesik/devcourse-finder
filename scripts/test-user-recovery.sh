#!/usr/bin/env bash
# Disposable PostgreSQL service only: synthetic accounts and throwaway keys.
set -euo pipefail
: "${TEST_POSTGRES_CONTAINER:?Dedicated test PostgreSQL container required}"
: "${TEST_DATABASE_URL:?Dedicated test PostgreSQL URL required}"
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
python3 - <<'PY'
import os,urllib.parse
u=urllib.parse.urlsplit(os.environ['TEST_DATABASE_URL'])
if not u.path.endswith('_test') or u.username!='devcourse' or u.password!='integration-test':
    raise SystemExit('Only the disposable integration-test database is supported')
PY
scratch=$(mktemp -d)
chmod 700 "$scratch"
source_created=false
restore_created=false
cleanup() {
  if $restore_created; then docker exec "$TEST_POSTGRES_CONTAINER" dropdb --force -U devcourse devcourse_auth_restored_test >/dev/null; fi
  if $source_created; then docker exec "$TEST_POSTGRES_CONTAINER" dropdb --force -U devcourse devcourse_auth_recovery_test >/dev/null; fi
  rm -rf -- "$scratch"
}
trap cleanup EXIT
original_url=$TEST_DATABASE_URL
fixture_url() {
  TEST_ORIGINAL_DATABASE_URL="$original_url" python3 - "$1" <<'PY'
import os,sys,urllib.parse
u=urllib.parse.urlsplit(os.environ['TEST_ORIGINAL_DATABASE_URL'])
print(urllib.parse.urlunsplit(u._replace(path='/'+sys.argv[1])))
PY
}
docker exec "$TEST_POSTGRES_CONTAINER" createdb -U devcourse devcourse_auth_recovery_test
source_created=true
mkdir -m 700 "$scratch/keys" "$scratch/restored-keys"
export TEST_DATABASE_URL
TEST_DATABASE_URL=$(fixture_url devcourse_auth_recovery_test)
AUTH_RECOVERY_STAGE=seed AUTH_RECOVERY_KEYS="$scratch/keys" go test ./backend/store -run '^TestPostgresCredentialRecovery$' -count=1
docker exec "$TEST_POSTGRES_CONTAINER" pg_dump -Fc -U devcourse -d devcourse_auth_recovery_test > "$scratch/database.dump"
cp -p "$scratch/keys/password-keyring.json" "$scratch/keys/password-recovery.json" "$scratch/keys/action-token.fixture" "$scratch/restored-keys/"
docker exec "$TEST_POSTGRES_CONTAINER" createdb -U devcourse devcourse_auth_restored_test
restore_created=true
docker exec -i "$TEST_POSTGRES_CONTAINER" pg_restore --exit-on-error -U devcourse -d devcourse_auth_restored_test < "$scratch/database.dump"
python3 - <<'PYRECOVERY'
import importlib.util,os,pathlib
path=pathlib.Path('scripts/disaster-recovery/recovery.py')
spec=importlib.util.spec_from_file_location('recovery',path);module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
module.invalidate_restored_account_actions(os.environ['TEST_POSTGRES_CONTAINER'],'devcourse_auth_restored_test')
PYRECOVERY
TEST_DATABASE_URL=$(fixture_url devcourse_auth_restored_test)
AUTH_RECOVERY_STAGE=verify AUTH_RECOVERY_KEYS="$scratch/restored-keys" go test ./backend/store -run '^TestPostgresCredentialRecovery$' -count=1
printf '%s\n' 'PASS: dumped/restored PostgreSQL credentials verify with restored old/new pepper keys'
