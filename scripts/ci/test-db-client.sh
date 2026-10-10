#!/usr/bin/env bash
set -Eeuo pipefail
set +x
set +a
umask 077

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
task_tmp=$(mktemp -d "${TMPDIR:-/tmp}/labtether-db-client-test.XXXXXX")
trap 'rm -rf -- "$task_tmp"' EXIT INT TERM HUP
mkdir "$task_tmp/bin" "$task_tmp/capture"
export DB_CLIENT_CAPTURE="$task_tmp/capture"
export DB_CLIENT_HELPER="${PROJECT_ROOT}/scripts/lib/db-client.py"
export DB_CLIENT_FIXTURE="$task_tmp/client-fixture.py"

# The executable is a fixture, but connection resolution below is real libpq.
# This catches PGDATABASE containing a URI: libpq treats it as a literal name
# and resolves the wrong user/host instead of expanding the URI parameters.
cat >"$DB_CLIENT_FIXTURE" <<'PY'
import importlib.util
import os
from pathlib import Path
import stat
import sys

spec = importlib.util.spec_from_file_location("db_client", os.environ["DB_CLIENT_HELPER"])
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)
client = sys.argv[1]
secret = "database:process\\secret "
assert all(secret not in item and "postgresql://" not in item for item in sys.argv)
assert all(secret not in item and "postgresql://" not in item for item in os.environ.values())
assert not any(key in os.environ for key in ("DATABASE_URL", "PGDATABASE", "PGPASSWORD"))
service = Path(os.environ["PGSERVICEFILE"])
assert stat.S_IMODE(service.stat().st_mode) == 0o600
assert stat.S_IMODE(service.parent.stat().st_mode) == 0o700
library = helper.load_libpq()
options = library.PQconndefaults()
assert options
resolved = {}
try:
    i = 0
    while options[i].keyword:
        resolved[options[i].keyword.decode()] = options[i].val
        i += 1
finally:
    library.PQconninfoFree(options)
assert resolved["host"] == b"/tmp/ltqa-socket space"
assert resolved["user"] == b"labtether_qa"
assert resolved["dbname"] == b"uri db"
assert resolved["sslmode"] == b"verify-full"
assert resolved["application_name"] == b" qa roundtrip"
passfile = Path(os.fsdecode(resolved["passfile"]))
assert stat.S_IMODE(passfile.stat().st_mode) == 0o600
assert passfile.read_bytes() == b"*:*:*:*:database\\:process\\\\secret \n"
capture = Path(os.environ["DB_CLIENT_CAPTURE"])
(capture / (client + "-service-path")).write_text(str(service))
if os.environ.get("DB_CLIENT_FAIL") == "1":
    sys.exit(23)
if client == "pg_dump":
    assert "--no-password" in sys.argv
    print("-- safe test backup")
else:
    assert "--no-psqlrc" in sys.argv and "--no-password" in sys.argv
    assert "--set=ON_ERROR_STOP=on" in sys.argv
    assert sys.stdin.read() == "SELECT 1;\n"
PY
for client in pg_dump psql; do
  cat >"$task_tmp/bin/$client" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail
exec python3 "$DB_CLIENT_FIXTURE" "${0##*/}" "$@"
SH
  chmod 700 "$task_tmp/bin/$client"
done
export PATH="$task_tmp/bin:$PATH"

# Query options override URI authority/path; percent encoding must use libpq's
# parser, including escaped password punctuation and a trailing space.
# Service values preserve leading whitespace after '='; check that with libpq.
database_fixture='postgresql://ignored:database%3Aprocess%5Csecret%20@ignored.invalid:6543/wrong?host=%2Ftmp%2Fltqa-socket%20space&user=labtether_qa&dbname=uri%20db&sslmode=verify-full&application_name=%20qa%20roundtrip'
assert_cleaned() {
  local service_path
  service_path=$(cat "$DB_CLIENT_CAPTURE/$1-service-path")
  [[ ! -e "${service_path%/*}" ]] || { echo 'FAIL: private database files remain' >&2; exit 1; }
}
run_backup() {
  DATABASE_URL="$database_fixture" BACKUP_DIR="$task_tmp/backups" KEEP_DAYS=36500 ENV_FILE="$task_tmp/missing.env" \
    bash "$PROJECT_ROOT/scripts/db-backup.sh"
}
run_backup >"$task_tmp/backup-output" 2>&1
assert_cleaned pg_dump
backup_file=$(find "$task_tmp/backups" -type f -name 'labtether_*.sql.gz' -print -quit)
[[ -n "$backup_file" && "$(gzip -dc "$backup_file")" == '-- safe test backup' ]]
python3 - "$backup_file" <<'PY'
import os, stat, sys
assert stat.S_IMODE(os.stat(sys.argv[1]).st_mode) == 0o600
PY
printf 'SELECT 1;\n' >"$task_tmp/restore.sql"
gzip -c "$task_tmp/restore.sql" >"$task_tmp/restore.sql.gz"
for fixture in "$task_tmp/restore.sql" "$task_tmp/restore.sql.gz"; do
  DATABASE_URL="$database_fixture" ENV_FILE="$task_tmp/missing.env" \
    bash "$PROJECT_ROOT/scripts/db-restore.sh" --yes "$fixture" >"$task_tmp/restore-output" 2>&1
  assert_cleaned psql
done
if DB_CLIENT_FAIL=1 run_backup >"$task_tmp/failure-output" 2>&1; then
  echo 'FAIL: failed database client reported success' >&2
  exit 1
fi
assert_cleaned pg_dump
[[ "$(find "$task_tmp/backups" -type f | wc -l | tr -d ' ')" == 1 ]]
for output in "$task_tmp/backup-output" "$task_tmp/restore-output" "$task_tmp/failure-output"; do
  if grep -F 'database%3Aprocess' "$output" >/dev/null || grep -F 'database:process' "$output" >/dev/null; then
    echo 'FAIL: database credentials appeared in output' >&2
    exit 1
  fi
done
echo 'PASS: backup and restore use actual libpq URI semantics with private credentials and cleanup'
