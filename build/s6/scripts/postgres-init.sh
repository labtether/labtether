#!/bin/sh
set -eu
umask 077

# Wait for PostgreSQL to be ready, then set up the database, role, and password.
# Connects via unix socket (auth-local=trust) to avoid password prompts.
validate_identifier() {
  identifier_name=$1
  identifier_value=$2
  if ! printf '%s' "$identifier_value" | LC_ALL=C grep -Eq '^[A-Za-z_][A-Za-z0-9_]{0,62}$'; then
    echo "postgres-init: $identifier_name must be a safe PostgreSQL identifier" >&2
    exit 1
  fi
}
validate_identifier POSTGRES_USER "${POSTGRES_USER:-}"
validate_identifier POSTGRES_DB "${POSTGRES_DB:-}"

config_dir=/data/config
if [ -L "$config_dir" ] || { [ -e "$config_dir" ] && [ ! -d "$config_dir" ]; }; then
  echo "postgres-init: refusing unsafe password directory" >&2
  exit 1
fi
mkdir -p "$config_dir"
chown labtether:labtether "$config_dir"
chmod 0700 "$config_dir"

password_file=$config_dir/postgres-password
if [ -L "$password_file" ] || { [ -e "$password_file" ] && [ ! -f "$password_file" ]; }; then
  echo "postgres-init: refusing unsafe password file" >&2
  exit 1
fi
if [ -f "$password_file" ]; then
  expected_owner="$(id -u labtether):$(id -g labtether)"
  actual_owner="$(stat -c '%u:%g' "$password_file")"
  actual_mode="$(stat -c '%a' "$password_file")"
  actual_links="$(stat -c '%h' "$password_file")"
  actual_size="$(wc -c < "$password_file" | tr -d '[:space:]')"
  PG_PASS="$(cat "$password_file")"
  if [ "$actual_owner" != "$expected_owner" ] || [ "$actual_mode" != 400 ] || [ "$actual_links" != 1 ]; then
    echo "postgres-init: password file ownership, mode, or link count is unsafe" >&2
    exit 1
  fi
  if [ "$actual_size" -ne 44 ] || [ "${#PG_PASS}" -ne 44 ] || ! printf '%s' "$PG_PASS" | LC_ALL=C grep -Eq '^[A-Za-z0-9_-]{43}=$'; then
    echo "postgres-init: password file content is invalid" >&2
    exit 1
  fi
fi

for _ in $(seq 1 30); do
  if /command/s6-setuidgid postgres pg_isready -h /run/postgresql -q 2>/dev/null; then
    # Create role if it doesn't exist
    /command/s6-setuidgid postgres psql -v ON_ERROR_STOP=1 -h /run/postgresql -tc "SELECT 1 FROM pg_roles WHERE rolname='${POSTGRES_USER}'" | grep -q 1 || \
      /command/s6-setuidgid postgres psql -v ON_ERROR_STOP=1 -h /run/postgresql -c "CREATE ROLE ${POSTGRES_USER} WITH LOGIN;"
    # Create database if it doesn't exist
    /command/s6-setuidgid postgres psql -h /run/postgresql -tc "SELECT 1 FROM pg_database WHERE datname='${POSTGRES_DB}'" | grep -q 1 || \
      /command/s6-setuidgid postgres createdb -h /run/postgresql -O "${POSTGRES_USER}" "${POSTGRES_DB}"
    # Generate and store password on first boot
    if [ ! -f "$password_file" ]; then
      PG_PASS=$(dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 | tr -d '\n' | tr '+/' '-_')
      password_tmp=$(mktemp /data/config/.postgres-password.XXXXXX)
      trap 'rm -f "$password_tmp"' EXIT HUP INT TERM
      printf '%s' "${PG_PASS}" > "$password_tmp"
      chmod 0400 "$password_tmp"
      chown labtether:labtether "$password_tmp"
      mv "$password_tmp" "$password_file"
      trap - EXIT HUP INT TERM
    fi
    # Re-apply the persisted password on every boot. This makes startup
    # recoverable if a prior run persisted the file but was interrupted before
    # ALTER ROLE, or if a database restore carries an older role password.
    PG_PASS="$(cat "$password_file")"
    /command/s6-setuidgid postgres psql -v ON_ERROR_STOP=1 -h /run/postgresql -c "ALTER ROLE ${POSTGRES_USER} PASSWORD '${PG_PASS}';"
    exit 0
  fi
  sleep 1
done
echo "PostgreSQL did not become ready in 30 seconds"
exit 1
