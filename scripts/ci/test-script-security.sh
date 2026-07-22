#!/usr/bin/env bash
set -Eeuo pipefail
set +x
umask 077

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/smoke-common.sh"

pass_count=0
fail_count=0

pass() {
  printf 'PASS: %s\n' "$1"
  pass_count=$((pass_count + 1))
}

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  fail_count=$((fail_count + 1))
}

assert_file_contains() {
  local file=$1
  local expected=$2
  local label=$3
  if grep -F -- "$expected" "$file" >/dev/null 2>&1; then
    pass "$label"
  else
    fail "$label"
  fi
}

assert_file_excludes() {
  local file=$1
  local forbidden=$2
  local label=$3
  if grep -F -- "$forbidden" "$file" >/dev/null 2>&1; then
    fail "$label"
  else
    pass "$label"
  fi
}

file_mode() {
  local file=$1
  if stat -f '%Lp' "$file" >/dev/null 2>&1; then
    stat -f '%Lp' "$file"
  else
    stat -c '%a' "$file"
  fi
}

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/labtether-script-security.XXXXXX")
cleanup() {
  labtether_cleanup_curl_security
  rm -rf -- "$tmp_dir"
}
trap cleanup EXIT INT TERM HUP

secret_env="${tmp_dir}/secrets.env"
printf 'LABTETHER_API_TOKEN=test-only\n' >"$secret_env"
chmod 644 "$secret_env"
if labtether_require_private_env_file "$secret_env" >/dev/null 2>&1; then
  fail "group-readable secret env is rejected"
else
  pass "group-readable secret env is rejected"
fi
chmod 600 "$secret_env"
if labtether_require_private_env_file "$secret_env" >/dev/null 2>&1; then
  pass "mode-0600 secret env is accepted"
else
  fail "mode-0600 secret env is accepted"
fi

dotenv_marker="${tmp_dir}/dotenv-command-executed"
dotenv_literal="${tmp_dir}/literal.env"
# shellcheck disable=SC2016 # The parser must preserve this literal expansion syntax.
printf '%s\n' \
  "LABTETHER_API_TOKEN=\$(touch ${dotenv_marker})" \
  'LABTETHER_API_BASE_URL="https://literal.example/api"' \
  'IGNORED_VALUE=${HOME}' >"$dotenv_literal"
chmod 600 "$dotenv_literal"
dotenv_value=""
if labtether_read_env_value dotenv_value "$dotenv_literal" LABTETHER_API_TOKEN >/dev/null 2>&1 && \
  [[ "$dotenv_value" == "\$(touch ${dotenv_marker})" && ! -e "$dotenv_marker" ]]; then
  pass "dotenv values are parsed literally without shell execution"
else
  fail "dotenv values are parsed literally without shell execution"
fi
if labtether_read_env_value dotenv_value "$dotenv_literal" LABTETHER_API_BASE_URL >/dev/null 2>&1 && \
  [[ "$dotenv_value" == 'https://literal.example/api' ]]; then
  pass "allowlisted quoted dotenv values are decoded"
else
  fail "allowlisted quoted dotenv values are decoded"
fi
unset LABTETHER_API_BASE_URL
if labtether_load_env_file_literals "$dotenv_literal" '^LABTETHER_API_BASE_URL$' >/dev/null 2>&1 && \
  [[ "$LABTETHER_API_BASE_URL" == 'https://literal.example/api' && ! -e "$dotenv_marker" ]]; then
  pass "dotenv loader exports only allowlisted literal values"
else
  fail "dotenv loader exports only allowlisted literal values"
fi
unset LABTETHER_API_BASE_URL

secret_file="${tmp_dir}/token.txt"
printf '%s\n' 'file-token-5c6f08' >"$secret_file"
chmod 600 "$secret_file"
secret_file_value=""
if labtether_read_private_secret_file secret_file_value "$secret_file" "test token file" >/dev/null 2>&1 && [[ "$secret_file_value" == 'file-token-5c6f08' ]]; then
  pass "mode-0600 single-line token file is read safely"
else
  fail "mode-0600 single-line token file is read safely"
fi
secret_link="${tmp_dir}/token-link.txt"
ln -s "$secret_file" "$secret_link"
if labtether_read_private_secret_file secret_file_value "$secret_link" "test token file" >/dev/null 2>&1; then
  fail "symlinked token file is rejected"
else
  pass "symlinked token file is rejected"
fi

private_output="${tmp_dir}/private-report.txt"
if labtether_prepare_private_output_file "$private_output" >/dev/null 2>&1 && [[ "$(file_mode "$private_output")" == "600" ]]; then
  pass "private output files are created without clobbering at mode 0600"
else
  fail "private output files are created without clobbering at mode 0600"
fi
printf '%s\n' 'preserve-existing-output' >"$private_output"
if labtether_prepare_private_output_file "$private_output" >/dev/null 2>&1 || [[ "$(<"$private_output")" != 'preserve-existing-output' ]]; then
  fail "existing output files are never overwritten"
else
  pass "existing output files are never overwritten"
fi
output_link="${tmp_dir}/private-report-link.txt"
ln -s "$private_output" "$output_link"
if labtether_prepare_private_output_file "$output_link" >/dev/null 2>&1; then
  fail "symlinked output paths are rejected"
else
  pass "symlinked output paths are rejected"
fi
private_run_dir=""
if labtether_make_private_run_dir private_run_dir "$tmp_dir" 'security-run' >/dev/null 2>&1 && \
  [[ -d "$private_run_dir" && "$(file_mode "$private_run_dir")" == "700" ]]; then
  pass "private run directories use unpredictable names and mode 0700"
else
  fail "private run directories use unpredictable names and mode 0700"
fi
if labtether_make_private_run_dir private_run_dir "$tmp_dir" '../unsafe-prefix' >/dev/null 2>&1; then
  fail "unsafe run-directory prefixes are rejected"
else
  pass "unsafe run-directory prefixes are rejected"
fi

login_json=""
if labtether_build_login_json login_json 'admin"name' 'pass\word"value' >/dev/null 2>&1 && \
  [[ "$login_json" == '{"username":"admin\"name","password":"pass\\word\"value"}' ]]; then
  pass "login JSON is escaped without passing the password to a child process"
else
  fail "login JSON is escaped without passing the password to a child process"
fi

redacted_json=$(labtether_redact_json_for_log '{"ticket":"ticket-secret-41","nested":{"vnc_password":"vnc-secret-62","safe":"visible"},"items":[{"apiToken":"token-secret-73"}],"stream_path":"/ws?ticket=query-secret-95"}')
if [[ "$redacted_json" == *'"safe":"visible"'* && "$redacted_json" == *'"ticket":"<redacted>"'* ]]; then
  pass "diagnostic JSON keeps non-sensitive fields while redacting credentials"
else
  fail "diagnostic JSON keeps non-sensitive fields while redacting credentials"
fi
if [[ "$redacted_json" == *'ticket-secret-41'* || "$redacted_json" == *'vnc-secret-62'* || "$redacted_json" == *'token-secret-73'* || "$redacted_json" == *'query-secret-95'* ]]; then
  fail "diagnostic JSON never contains nested credential values"
else
  pass "diagnostic JSON never contains nested credential values"
fi
invalid_json_secret='invalid-response-secret-84'
if [[ "$(labtether_redact_json_for_log "not-json-${invalid_json_secret}")" == *"$invalid_json_secret"* ]]; then
  fail "invalid diagnostic responses are omitted instead of echoed"
else
  pass "invalid diagnostic responses are omitted instead of echoed"
fi

capture_dir="${tmp_dir}/capture"
fake_bin="${tmp_dir}/bin"
mkdir -p "$capture_dir" "$fake_bin"
cat >"${fake_bin}/curl" <<'FAKE_CURL'
#!/usr/bin/env bash
set -Eeuo pipefail
: "${CURL_CAPTURE_DIR:?}"
printf '%s\n' "$@" >"${CURL_CAPTURE_DIR}/argv"
env >"${CURL_CAPTURE_DIR}/environment"
cat >"${CURL_CAPTURE_DIR}/stdin"
printf '{}\n200'
FAKE_CURL
chmod 700 "${fake_bin}/curl"

token="stub-token-7f559db9d1"
payload='{"password":"request-body-only-3f45ab"}'
export AUTH_TOKEN="$token"
export LABTETHER_API_TOKEN="$token"
export LABTETHER_OWNER_TOKEN="$token"
export EXTERNAL_API_TOKEN="$token"
export EXTERNAL_OWNER_TOKEN="$token"
labtether_prepare_curl_auth "$AUTH_TOKEN"
auth_config="$LABTETHER_CURL_AUTH_CONFIG_FILE"
labtether_clear_token_environment

if [[ "$(file_mode "$LABTETHER_SECURE_CURL_DIR")" == "700" ]]; then
  pass "curl security directory is mode 0700"
else
  fail "curl security directory is mode 0700"
fi
if [[ "$(file_mode "$auth_config")" == "600" ]]; then
  pass "curl auth config is mode 0600"
else
  fail "curl auth config is mode 0600"
fi

ca_file="${tmp_dir}/test-ca.pem"
printf '%s\n' 'test-only-ca-placeholder' >"$ca_file"
# Read by the sourced script-common request helper.
# shellcheck disable=SC2034
LABTETHER_CA_FILE="$ca_file"
# Read by the sourced script-common request helper.
# shellcheck disable=SC2034
LABTETHER_INSECURE_TLS=0
export CURL_CAPTURE_DIR="$capture_dir"
export CURL_CA_BUNDLE="${tmp_dir}/ambient-curl-ca"
export SSL_CERT_FILE="${tmp_dir}/ambient-ssl-cert"
export SSL_CERT_DIR="${tmp_dir}/ambient-ssl-dir"
export SSLKEYLOGFILE="${tmp_dir}/ambient-keylog"
export CURL_HOME="${tmp_dir}/ambient-curl-home"
export NETRC="${tmp_dir}/ambient-netrc"
export CURL_SSL_BACKEND="ambient-backend"
PATH="${fake_bin}:$PATH"

response_body=""
response_status=""
run_request response_body response_status POST "https://labtether.invalid/api" "$payload" 1
if [[ "$response_status" == "200" && "$response_body" == "{}" ]]; then
  pass "stubbed authenticated request completes"
else
  fail "stubbed authenticated request completes"
fi
assert_file_contains "${capture_dir}/argv" '--config' "authenticated curl uses a config file"
if [[ "$(head -n 1 "${capture_dir}/argv")" == '--disable' ]]; then
  pass "curl disables ambient default configuration first"
else
  fail "curl disables ambient default configuration first"
fi
assert_file_contains "${capture_dir}/argv" "$auth_config" "authenticated curl references the secure config"
assert_file_contains "${capture_dir}/argv" '--cacert' "authenticated HTTPS uses the selected CA"
assert_file_contains "${capture_dir}/argv" '--noproxy' "curl bypasses ambient proxies by default"
assert_file_contains "${capture_dir}/argv" '--data-binary' "request bodies use data-binary"
assert_file_contains "${capture_dir}/argv" '@-' "request bodies are read from stdin"
assert_file_excludes "${capture_dir}/argv" "$token" "token is absent from curl argv"
assert_file_excludes "${capture_dir}/environment" "$token" "token is absent from the curl environment"
assert_file_excludes "${capture_dir}/argv" 'request-body-only-3f45ab' "request body is absent from curl argv"
assert_file_excludes "${capture_dir}/environment" 'request-body-only-3f45ab' "request body is absent from the curl environment"
for ambient_name in CURL_CA_BUNDLE SSL_CERT_FILE SSL_CERT_DIR SSLKEYLOGFILE CURL_HOME NETRC CURL_SSL_BACKEND; do
  assert_file_excludes "${capture_dir}/environment" "${ambient_name}=" "ambient ${ambient_name} is removed from curl"
done
assert_file_contains "${capture_dir}/stdin" 'request-body-only-3f45ab' "request body is delivered on stdin"
assert_file_excludes "${capture_dir}/argv" '--insecure' "verified request does not use insecure TLS"

ca_link="${tmp_dir}/test-ca-link.pem"
ln -s "$ca_file" "$ca_link"
LABTETHER_CA_FILE="$ca_link"
if labtether_validate_tls_options >/dev/null 2>&1; then
  fail "symlinked TLS CA files are rejected"
else
  pass "symlinked TLS CA files are rejected"
fi
# Read by the sourced script-common TLS validator.
# shellcheck disable=SC2034
LABTETHER_CA_FILE="$ca_file"
chmod 666 "$ca_file"
if labtether_validate_tls_options >/dev/null 2>&1; then
  fail "group/other-writable TLS CA files are rejected"
else
  pass "group/other-writable TLS CA files are rejected"
fi
chmod 600 "$ca_file"

unset LABTETHER_CA_FILE
LABTETHER_INSECURE_TLS=1
if labtether_build_curl_request_args "https://labtether.invalid/api" 1 >/dev/null 2>&1; then
  fail "authenticated insecure TLS is rejected"
else
  pass "authenticated insecure TLS is rejected"
fi

# Read by the sourced script-common request helper.
# shellcheck disable=SC2034
LABTETHER_INSECURE_TLS=0
if labtether_build_curl_request_args "http://192.0.2.10/api" 1 >/dev/null 2>&1; then
  fail "authenticated non-loopback cleartext HTTP is rejected"
else
  pass "authenticated non-loopback cleartext HTTP is rejected"
fi
if labtether_build_curl_request_args "http://192.0.2.10/auth/login" 0 1 >/dev/null 2>&1; then
  fail "sensitive login over non-loopback cleartext HTTP is rejected"
else
  pass "sensitive login over non-loopback cleartext HTTP is rejected"
fi
if labtether_build_curl_request_args "HtTp://192.0.2.10/api" 1 >/dev/null 2>&1; then
  fail "mixed-case non-loopback cleartext HTTP is rejected"
else
  pass "mixed-case non-loopback cleartext HTTP is rejected"
fi
if labtether_build_curl_request_args "WS://192.0.2.10/stream" 1 >/dev/null 2>&1; then
  fail "authenticated non-loopback cleartext WebSocket is rejected"
else
  pass "authenticated non-loopback cleartext WebSocket is rejected"
fi
if labtether_build_curl_request_args "file:///tmp/credential-sink" 1 >/dev/null 2>&1; then
  fail "authenticated unsupported URL schemes are rejected"
else
  pass "authenticated unsupported URL schemes are rejected"
fi
if labtether_build_curl_request_args "http://127.0.0.1:8080/api" 1 >/dev/null 2>&1; then
  pass "authenticated loopback HTTP remains available for local development"
else
  fail "authenticated loopback HTTP remains available for local development"
fi
if labtether_build_curl_request_args "HTTP://LOCALHOST:8080/api" 1 >/dev/null 2>&1; then
  pass "mixed-case loopback HTTP remains available for local development"
else
  fail "mixed-case loopback HTTP remains available for local development"
fi
if labtether_prepare_curl_auth $'bad-token\nheader-injection' >/dev/null 2>&1; then
  fail "control characters in authentication tokens are rejected"
else
  pass "control characters in authentication tokens are rejected"
fi

# Read by the sourced script-common request helper.
# shellcheck disable=SC2034
LABTETHER_ALLOW_PROXY=1
labtether_build_curl_request_args "https://labtether.invalid/api" 1
if printf '%s\n' "${LABTETHER_CURL_REQUEST_ARGS[@]}" | grep -Fx -- '--noproxy' >/dev/null 2>&1; then
  fail "explicit proxy opt-in removes the noproxy default"
else
  pass "explicit proxy opt-in removes the noproxy default"
fi
unset LABTETHER_ALLOW_PROXY

cli_secret='cli-secret-must-not-be-echoed-9e72'
for perf_script in \
  "${PROJECT_ROOT}/scripts/perf/profile.sh" \
  "${PROJECT_ROOT}/scripts/perf/baseline.sh" \
  "${PROJECT_ROOT}/scripts/perf/backend-hotspot-apples.sh"; do
  for secret_flag in --token --password; do
    parse_output=""
    if parse_output=$("$perf_script" "$secret_flag" "$cli_secret" 2>&1); then
      fail "$(basename "$perf_script") rejects secret-valued ${secret_flag}"
    else
      pass "$(basename "$perf_script") rejects secret-valued ${secret_flag}"
    fi
    if [[ "$parse_output" == *"$cli_secret"* ]]; then
      fail "$(basename "$perf_script") does not echo rejected ${secret_flag} values"
    else
      pass "$(basename "$perf_script") does not echo rejected ${secret_flag} values"
    fi
  done
done

for dotenv_consumer in \
  "${PROJECT_ROOT}/scripts/smoke-test.sh" \
  "${PROJECT_ROOT}/scripts/desktop-smoke-test.sh" \
  "${PROJECT_ROOT}/scripts/capture-vnc-repro.sh" \
  "${PROJECT_ROOT}/scripts/integration-queue-flow.sh" \
  "${PROJECT_ROOT}/scripts/db-migrate.sh" \
  "${PROJECT_ROOT}/scripts/db-migrate-status.sh" \
  "${PROJECT_ROOT}/scripts/db-restore.sh" \
  "${PROJECT_ROOT}/scripts/db-backup.sh" \
  "${PROJECT_ROOT}/scripts/dev-backend-bg.sh" \
  "${PROJECT_ROOT}/scripts/dev-backend-run.sh"; do
  # shellcheck disable=SC2016 # This is the literal unsafe source expression being searched for.
  assert_file_excludes "$dotenv_consumer" 'source "${ENV_FILE}"' "$(basename "$dotenv_consumer") does not execute dotenv as shell code"
done

assert_file_contains "${PROJECT_ROOT}/scripts/dev-frontend-bg.sh" 'shell_join_quoted frontend_shell_command' "frontend tmux command shell-quotes every argument"
assert_file_contains "${PROJECT_ROOT}/scripts/dev-frontend-bg.sh" 'LABTETHER_FRONTEND_BIND:-127.0.0.1' "development frontend binds to loopback by default"
# shellcheck disable=SC2016 # This is the literal PID-file guard being searched for.
assert_file_contains "${PROJECT_ROOT}/scripts/dev-backend-bg.sh" 'labtether_lock_down_private_file "$PID_FILE"' "background backend validates PID-file ownership and permissions"
assert_file_contains "${PROJECT_ROOT}/scripts/dev-backend-run.sh" 'labtether_load_env_file_literals' "development backend loads dotenv values without shell evaluation"
assert_file_contains "${PROJECT_ROOT}/scripts/fetch-dashboard-icons.sh" 'DASHBOARD_ICONS_SHA256' "dashboard icon archives require a pinned digest"
assert_file_contains "${PROJECT_ROOT}/scripts/bootstrap.sh" 'Admin password source: LABTETHER_ADMIN_PASSWORD in the private' "bootstrap points to the private env file instead of printing generated credentials"
# shellcheck disable=SC2016 # This is the literal legacy output expression being searched for.
assert_file_excludes "${PROJECT_ROOT}/scripts/db-restore.sh" 'echo "Target database: ${DB_URL}"' "database restore never prints a credential-bearing URL"
# shellcheck disable=SC2016 # This is the literal legacy pg_dump invocation being searched for.
assert_file_excludes "${PROJECT_ROOT}/scripts/db-backup.sh" 'pg_dump "$DB_URL"' "database backup keeps credential-bearing URLs out of process arguments"

db_capture_dir="${tmp_dir}/database-capture"
mkdir -p "$db_capture_dir"
cat >"${fake_bin}/pg_dump" <<'FAKE_PG_DUMP'
#!/usr/bin/env bash
set -Eeuo pipefail
: "${DB_CAPTURE_DIR:?}"
printf '%s\n' "$@" >"${DB_CAPTURE_DIR}/pg-dump-argv"
env >"${DB_CAPTURE_DIR}/pg-dump-environment"
printf '%s\n' '-- safe test backup'
FAKE_PG_DUMP
cat >"${fake_bin}/psql" <<'FAKE_PSQL'
#!/usr/bin/env bash
set -Eeuo pipefail
: "${DB_CAPTURE_DIR:?}"
printf '%s\n' "$@" >"${DB_CAPTURE_DIR}/psql-argv"
env >"${DB_CAPTURE_DIR}/psql-environment"
cat >"${DB_CAPTURE_DIR}/psql-stdin"
FAKE_PSQL
chmod 700 "${fake_bin}/pg_dump" "${fake_bin}/psql"
export DB_CAPTURE_DIR="$db_capture_dir"
database_process_secret='postgres://labtether:database-process-secret-39@localhost:5432/labtether?sslmode=disable'
backup_output=""
if backup_output=$(DATABASE_URL="$database_process_secret" BACKUP_DIR="${tmp_dir}/backups" KEEP_DAYS=36500 ENV_FILE="${tmp_dir}/missing.env" \
  "${PROJECT_ROOT}/scripts/db-backup.sh" 2>&1); then
  pass "database backup completes with a stubbed pg_dump"
else
  fail "database backup completes with a stubbed pg_dump"
  printf '  backup diagnostic: %s\n' "${backup_output//$database_process_secret/<redacted>}" >&2
fi
assert_file_excludes "${db_capture_dir}/pg-dump-argv" "$database_process_secret" "database URL is absent from pg_dump argv"
assert_file_contains "${db_capture_dir}/pg-dump-environment" "PGDATABASE=${database_process_secret}" "database URL reaches pg_dump only through its environment"
if [[ "$backup_output" == *"$database_process_secret"* ]]; then
  fail "database backup output does not reveal the database URL"
else
  pass "database backup output does not reveal the database URL"
fi
backup_file=""
if [[ -d "${tmp_dir}/backups" ]]; then
  backup_file=$(find "${tmp_dir}/backups" -type f -name 'labtether_*.sql.gz' -print -quit)
fi
if [[ -n "$backup_file" && "$(file_mode "$backup_file")" == "600" && "$(gzip -dc "$backup_file")" == '-- safe test backup' ]]; then
  pass "database backups are private compressed files"
else
  fail "database backups are private compressed files"
fi

restore_fixture="${tmp_dir}/restore.sql"
printf '%s\n' 'SELECT 1;' >"$restore_fixture"
restore_output=""
if restore_output=$(DATABASE_URL="$database_process_secret" ENV_FILE="${tmp_dir}/missing.env" \
  "${PROJECT_ROOT}/scripts/db-restore.sh" --yes "$restore_fixture" 2>&1); then
  pass "database restore completes with a stubbed psql"
else
  fail "database restore completes with a stubbed psql"
fi
assert_file_excludes "${db_capture_dir}/psql-argv" "$database_process_secret" "database URL is absent from psql argv"
assert_file_contains "${db_capture_dir}/psql-environment" "PGDATABASE=${database_process_secret}" "database URL reaches psql only through its environment"
assert_file_contains "${db_capture_dir}/psql-argv" '--no-psqlrc' "database restore ignores user psql startup commands"
assert_file_contains "${db_capture_dir}/psql-stdin" 'SELECT 1;' "database restore streams the selected backup to psql"
if [[ "$restore_output" == *"$database_process_secret"* ]]; then
  fail "database restore output does not reveal the database URL"
else
  pass "database restore output does not reveal the database URL"
fi

assert_file_contains "${PROJECT_ROOT}/scripts/setup-authentik-test.sh" 'labtether-authentik-oidc.env.XXXXXX' "Authentik credentials use an unpredictable macOS-compatible temp file"

labtether_cleanup_curl_security
if [[ ! -e "$auth_config" ]]; then
  pass "curl auth config is removed during cleanup"
else
  fail "curl auth config is removed during cleanup"
fi

printf 'Script security checks: %d passed, %d failed\n' "$pass_count" "$fail_count"
[[ "$fail_count" -eq 0 ]]
