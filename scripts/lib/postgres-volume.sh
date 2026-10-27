#!/usr/bin/env bash
# LabTether v1.4.0 mounted postgres-data at /var/lib/postgresql/data, but
# PostgreSQL 18 keeps its cluster in /var/lib/postgresql/18/docker. Those
# databases therefore live in the postgres container's unnamed volume, which the
# current compose files (postgres-data at /var/lib/postgresql) never mount.
#
# labtether_move_legacy_postgres_volume copies that cluster into postgres-data
# once, before the new compose file starts PostgreSQL, and leaves the unnamed
# volume untouched as a fallback. Its arguments are the `docker compose`
# options for the install, for example --env-file .env.deploy -f <file>.
#
# The unnamed volume is found through the existing postgres container. If that
# container was already replaced (for example by a plain `docker compose up`
# with the new files), name the old volume in LABTETHER_POSTGRES_LEGACY_VOLUME.
# Requires log_info/log_pass/log_fail from script-common.sh.

labtether_move_legacy_postgres_volume() {
  local container project legacy_volume named_volume require_cluster=0
  legacy_volume="${LABTETHER_POSTGRES_LEGACY_VOLUME:-}"
  if [[ -n "${legacy_volume}" ]]; then
    require_cluster=1
    if ! docker volume inspect "${legacy_volume}" >/dev/null 2>&1; then
      log_fail "LABTETHER_POSTGRES_LEGACY_VOLUME names a volume that does not exist: ${legacy_volume}"
      return 1
    fi
  else
    container="$(docker compose "$@" ps --all --quiet postgres)" || return 1
    if [[ -z "${container}" ]]; then
      return 0
    fi
    if [[ "${container}" == *$'\n'* ]]; then
      log_fail "found more than one postgres container; remove the extra one before upgrading"
      return 1
    fi
    named_volume="$(docker inspect --format '{{range .Mounts}}{{if and (eq .Type "volume") (eq .Destination "/var/lib/postgresql/data")}}{{.Name}}{{end}}{{end}}' "${container}")" || return 1
    if [[ -z "${named_volume}" ]]; then
      # postgres-data is not mounted at the old path: already the PostgreSQL 18 layout.
      return 0
    fi
    project="$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "${container}")" || return 1
    legacy_volume="$(docker inspect --format '{{range .Mounts}}{{if and (eq .Type "volume") (eq .Destination "/var/lib/postgresql")}}{{.Name}}{{end}}{{end}}' "${container}")" || return 1
    if [[ -z "${project}" || "${named_volume}" != "${project}_postgres-data" || -z "${legacy_volume}" || "${legacy_volume}" == "${named_volume}" ]]; then
      log_fail "postgres container ${container} has an unexpected volume layout; not moving its data"
      return 1
    fi
  fi

  log_info "Moving the PostgreSQL database from volume ${legacy_volume} into postgres-data..."
  docker compose "$@" stop || return 1
  # The one-off container uses the new postgres service definition, so
  # /var/lib/postgresql is exactly the postgres-data volume the stack will use.
  # An empty old volume means PostgreSQL never stored data there; only the
  # marker is written and the new stack starts with an empty database.
  docker compose "$@" run --rm --no-deps --no-TTY --env "LABTETHER_REQUIRE_CLUSTER=${require_cluster}" \
    --volume "${legacy_volume}:/labtether-legacy-postgres:ro" --entrypoint /bin/sh postgres -ec '
      from=/labtether-legacy-postgres
      to=/var/lib/postgresql
      for existing in "$to"/PG_VERSION "$to"/*/docker/PG_VERSION; do
        if [ -e "$existing" ]; then
          echo "postgres-data already holds a database ($existing); not overwriting it" >&2
          exit 1
        fi
      done
      if [ -s "$from/18/docker/PG_VERSION" ]; then
        cp -a "$from/." "$to/"
        test -s "$to/18/docker/PG_VERSION"
      elif [ "$LABTETHER_REQUIRE_CLUSTER" = 1 ]; then
        echo "the named volume holds no PostgreSQL 18 database" >&2
        exit 1
      fi
      : > "$to/.labtether-postgres-volume"
    ' || {
    log_fail "could not move the database; volume ${legacy_volume} is unchanged"
    return 1
  }
  log_pass "Moved the database into postgres-data; volume ${legacy_volume} is kept as a backup."
}
