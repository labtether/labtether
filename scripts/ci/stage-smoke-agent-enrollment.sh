#!/usr/bin/env bash
set -Eeuo pipefail
# shellcheck source=/dev/null
source ./scripts/lib/script-common.sh
trap labtether_cleanup_curl_security EXIT INT TERM HUP
mkdir -p "${RUNNER_TEMP}/labtether-ca"
docker compose cp labtether:/ca/ca.crt "${RUNNER_TEMP}/labtether-ca/ca.crt"
export LABTETHER_CA_FILE="${RUNNER_TEMP}/labtether-ca/ca.crt"
labtether_prepare_curl_auth "${LABTETHER_API_TOKEN}"
labtether_clear_token_environment
enrollment_url=https://localhost:8443/settings/enrollment
labtether_build_curl_request_args "${enrollment_url}" 1 1
printf '%s\n' '{"label":"labtether-ci-agent","ttl_hours":1,"max_uses":1,"scope":"asset","asset_id":"labtether-ci-agent"}' |
  labtether_curl "${LABTETHER_CURL_REQUEST_ARGS[@]}" \
    --silent --show-error --fail-with-body \
    --request POST \
    --header 'Content-Type: application/json' \
    --data-binary @- \
    "${enrollment_url}" |
  jq -er 'select(.token.max_uses == 1 and (.token.id | type == "string" and length > 0)) | .raw_token | select(type == "string" and length > 0)' |
  COMPOSE_PROFILES=ci-agent docker compose run --rm --no-deps -T \
    --user 0:0 \
    --cap-add CHOWN \
    --entrypoint /bin/sh \
    qa-agent \
    -c '
      set -eu
      directory=/run/labtether-enrollment
      token_file="${directory}/enrollment-token"
      test ! -e "${token_file}"
      chmod 0700 "${directory}"
      umask 077
      cat > "${token_file}"
      test -s "${token_file}"
      chmod 0600 "${token_file}"
      chown 1001:1001 "${token_file}" "${directory}"
    '
