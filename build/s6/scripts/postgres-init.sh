#!/bin/sh
# Wait for PostgreSQL to be ready, then set up the database, role, and password.
# Connects via unix socket (auth-local=trust) to avoid password prompts.
for i in $(seq 1 30); do
  if /command/s6-setuidgid postgres pg_isready -h /run/postgresql -q 2>/dev/null; then
    # Create role if it doesn't exist
    /command/s6-setuidgid postgres psql -h /run/postgresql -tc "SELECT 1 FROM pg_roles WHERE rolname='${POSTGRES_USER}'" | grep -q 1 || \
      /command/s6-setuidgid postgres psql -h /run/postgresql -c "CREATE ROLE ${POSTGRES_USER} WITH LOGIN;"
    # Create database if it doesn't exist
    /command/s6-setuidgid postgres psql -h /run/postgresql -tc "SELECT 1 FROM pg_database WHERE datname='${POSTGRES_DB}'" | grep -q 1 || \
      /command/s6-setuidgid postgres createdb -h /run/postgresql -O "${POSTGRES_USER}" "${POSTGRES_DB}"
    # Generate and store password on first boot
    if [ ! -f /data/config/postgres-password ]; then
      PG_PASS=$(dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 | tr -d '\n' | tr '+/' '-_')
      printf '%s' "${PG_PASS}" > /data/config/postgres-password
      chmod 0400 /data/config/postgres-password
      chown labtether:labtether /data/config/postgres-password
      /command/s6-setuidgid postgres psql -h /run/postgresql -c "ALTER ROLE ${POSTGRES_USER} PASSWORD '${PG_PASS}';"
    fi
    exit 0
  fi
  sleep 1
done
echo "PostgreSQL did not become ready in 30 seconds"
exit 1
