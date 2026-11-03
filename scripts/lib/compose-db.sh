#!/usr/bin/env bash
# Database backup and restore through the Compose stack's own postgres
# container. Neither Compose stack publishes Postgres on the host, so the
# client tools run inside the container, as root over its local socket. The
# container's POSTGRES_USER and POSTGRES_DB name the database, so no password
# or connection string is needed on the host.
#
# Callers set LABTETHER_COMPOSE_FILE and LABTETHER_COMPOSE_ENV_FILE first.

labtether_compose_db_args=()

# labtether_compose_db_prepare checks the files and Docker, and fills
# labtether_compose_db_args. A release install keeps both files in its
# folder, so the Compose project is that folder, as when the installer ran.
labtether_compose_db_prepare() {
  local compose_file=$1 env_file=$2
  if [[ ! -f "$compose_file" || -L "$compose_file" ]]; then
    log_fail "Compose file not found: ${compose_file}"
    return 1
  fi
  if [[ ! -f "$env_file" || -L "$env_file" ]]; then
    log_fail "Compose env file not found: ${env_file}"
    return 1
  fi
  if ! docker compose version >/dev/null 2>&1; then
    log_fail "docker compose is required"
    return 1
  fi
  labtether_compose_db_args=(--env-file "$env_file" -f "$compose_file")
  if [[ -z "$(docker compose "${labtether_compose_db_args[@]}" ps --status running --quiet postgres 2>/dev/null)" ]]; then
    log_fail "the postgres service of this stack is not running; start the stack first"
    return 1
  fi
}

# labtether_compose_db_psql runs psql in the container on the given database
# ("" for the stack's own) with ON_ERROR_STOP; extra arguments follow.
labtether_compose_db_psql() {
  local database=$1
  shift
  docker compose "${labtether_compose_db_args[@]}" exec -T -e LABTETHER_RESTORE_DB="$database" postgres \
    sh -c 'exec psql --no-psqlrc --quiet --set=ON_ERROR_STOP=1 --username="$POSTGRES_USER" --dbname="${LABTETHER_RESTORE_DB:-$POSTGRES_DB}" "$@"' psql "$@"
}

# labtether_compose_db_dump writes a plain SQL dump of the stack's database to
# standard output.
labtether_compose_db_dump() {
  docker compose "${labtether_compose_db_args[@]}" exec -T postgres \
    sh -c 'exec pg_dump --no-password --username="$POSTGRES_USER" --dbname="$POSTGRES_DB"'
}

# labtether_compose_db_restore replaces the stack's database with the plain
# SQL dump on standard input: it drops and re-creates the database, so a dump
# without DROP statements loads into a stack that already has tables. The Hub
# is stopped first and started again after a good load, so nothing writes
# while the database is being replaced.
labtether_compose_db_restore() {
  local hub_was_running=0 restore_rc=0
  # Only the final psql reads the dump on standard input; every other
  # command gets /dev/null so docker cannot swallow part of the dump.
  if [[ -n "$(docker compose "${labtether_compose_db_args[@]}" ps --status running --quiet labtether </dev/null 2>/dev/null)" ]]; then
    hub_was_running=1
    log_info "Stopping the Hub so nothing writes during the restore..."
    docker compose "${labtether_compose_db_args[@]}" stop labtether </dev/null >/dev/null || return 1
  fi
  log_info "Replacing the database with an empty one..."
  # shellcheck disable=SC2016 # expanded inside the container
  docker compose "${labtether_compose_db_args[@]}" exec -T postgres sh -c '
    psql --no-psqlrc --quiet --set=ON_ERROR_STOP=1 --username="$POSTGRES_USER" --dbname=postgres \
      --set=db="$POSTGRES_DB" --set=owner="$POSTGRES_USER" <<"SQL"
DROP DATABASE IF EXISTS :"db" WITH (FORCE);
CREATE DATABASE :"db" OWNER :"owner";
SQL' </dev/null || return 1
  log_info "Loading the backup..."
  labtether_compose_db_psql "" || restore_rc=$?
  if ((restore_rc != 0)); then
    log_fail "The backup did not load. The Hub stays stopped; run the restore again once the cause is fixed."
    return "$restore_rc"
  fi
  if ((hub_was_running)); then
    log_info "Starting the Hub again..."
    docker compose "${labtether_compose_db_args[@]}" start labtether </dev/null >/dev/null || return 1
  fi
}
