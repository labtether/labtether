#!/usr/bin/env bash

# Shared curl security state. Keep credentials out of argv and the inherited
# environment: authenticated callers use a mode-0600 curl config file instead.
LABTETHER_SECURE_CURL_DIR=""
LABTETHER_CURL_AUTH_CONFIG_FILE=""
LABTETHER_CURL_REQUEST_ARGS=()

labtether_value_is_true() {
  case "$(printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]')" in
    1|true|yes|on)
      return 0
      ;;
  esac
  return 1
}

labtether_validate_tls_options() {
  if labtether_value_is_true "${LABTETHER_INSECURE_TLS:-0}" && [[ -n "${LABTETHER_CA_FILE:-}" ]]; then
    log_fail "LABTETHER_CA_FILE and insecure TLS mode are mutually exclusive"
    return 1
  fi
  if [[ -n "${LABTETHER_CA_FILE:-}" ]]; then
    if [[ ! -f "${LABTETHER_CA_FILE}" || ! -r "${LABTETHER_CA_FILE}" || -L "${LABTETHER_CA_FILE}" ]]; then
      log_fail "TLS CA file must be a readable, non-symlink regular file: ${LABTETHER_CA_FILE}"
      return 1
    fi
    local ca_owner=""
    local ca_mode=""
    ca_owner=$(labtether_file_owner_uid "${LABTETHER_CA_FILE}" 2>/dev/null || true)
    if stat -f '%Lp' "${LABTETHER_CA_FILE}" >/dev/null 2>&1; then
      ca_mode=$(stat -f '%Lp' "${LABTETHER_CA_FILE}")
    else
      ca_mode=$(stat -c '%a' "${LABTETHER_CA_FILE}" 2>/dev/null || true)
    fi
    if [[ "$ca_owner" != "$(id -u)" && "$ca_owner" != "0" ]]; then
      log_fail "TLS CA file must be owned by the current user or root: ${LABTETHER_CA_FILE}"
      return 1
    fi
    if [[ ! "$ca_mode" =~ ^[0-7]{3,4}$ || "${ca_mode: -2:1}" =~ [2367] || "${ca_mode: -1}" =~ [2367] ]]; then
      log_fail "TLS CA file must not be group/other writable: ${LABTETHER_CA_FILE}"
      return 1
    fi
  fi
}

# shellcheck source=scripts/lib/private-files.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/private-files.sh"

labtether_build_login_json() {
  local __payload_var=$1
  local username=$2
  local password=$3
  if [[ -z "$username" || "$username" =~ [[:cntrl:]] ]]; then
    log_fail "login username must be non-empty and contain no control characters"
    return 1
  fi
  if [[ "$password" =~ [[:cntrl:]] ]]; then
    log_fail "login password must contain no control characters"
    return 1
  fi

  local escaped_username=$username
  local escaped_password=$password
  escaped_username=${escaped_username//\\/\\\\}
  escaped_username=${escaped_username//\"/\\\"}
  escaped_password=${escaped_password//\\/\\\\}
  escaped_password=${escaped_password//\"/\\\"}
  printf -v "$__payload_var" '{"username":"%s","password":"%s"}' "$escaped_username" "$escaped_password"
}

# Render a JSON response for diagnostic output while recursively replacing
# credential-bearing fields. Invalid JSON is never echoed back because it may
# itself contain an unstructured secret or an upstream error page.
labtether_redact_json_for_log() {
  local json=${1:-}
  local redacted=""
  if ! command -v jq >/dev/null 2>&1; then
    printf '%s' '[response omitted because jq is unavailable]'
    return 0
  fi
  if redacted=$(printf '%s' "$json" | jq -c '
    def redact:
      if type == "object" then
        with_entries(
          if (.key | test("password|ticket|token|secret|credential|authorization|cookie"; "i"))
          then .value = "<redacted>"
          else .value |= redact
          end
        )
      elif type == "array" then map(redact)
      elif type == "string" and test("(^|[?&])(password|ticket|token|secret|credential|authorization|cookie)=|^bearer[[:space:]]"; "i") then
        "<redacted>"
      else .
      end;
    redact
  ' 2>/dev/null); then
    printf '%s' "$redacted"
  else
    printf '%s' '[response omitted because it was not valid JSON]'
  fi
}

labtether_ensure_secure_curl_dir() {
  if [[ -n "${LABTETHER_SECURE_CURL_DIR}" ]]; then
    return 0
  fi
  local old_umask
  old_umask=$(umask)
  umask 077
  LABTETHER_SECURE_CURL_DIR=$(mktemp -d "${TMPDIR:-/tmp}/labtether-curl.XXXXXX")
  local rc=$?
  umask "$old_umask"
  if [[ $rc -ne 0 || -z "${LABTETHER_SECURE_CURL_DIR}" ]]; then
    log_fail "failed to create secure curl temporary directory"
    return 1
  fi
  chmod 700 "${LABTETHER_SECURE_CURL_DIR}"
}

labtether_prepare_curl_auth() {
  local token=${1:-}
  # Bash local variables inherit an existing global export attribute. Ensure a
  # caller's exported lowercase `token` cannot turn this local secret into a
  # child-process environment variable while mktemp/chmod run.
  export -n token 2>/dev/null || true
  if [[ -z "$token" ]]; then
    log_fail "cannot initialize authenticated curl without a token"
    return 1
  fi
  if [[ "$token" =~ [[:cntrl:]] ]]; then
    log_fail "API token contains an invalid control character"
    return 1
  fi
  labtether_ensure_secure_curl_dir || return 1

  local escaped=$token
  export -n escaped 2>/dev/null || true
  escaped=${escaped//\\/\\\\}
  escaped=${escaped//\"/\\\"}
  LABTETHER_CURL_AUTH_CONFIG_FILE="${LABTETHER_SECURE_CURL_DIR}/auth.curlrc"
  (umask 077; printf 'header = "Authorization: Bearer %s"\n' "$escaped" >"${LABTETHER_CURL_AUTH_CONFIG_FILE}") || {
    log_fail "failed to create secure curl authentication config"
    return 1
  }
  chmod 600 "${LABTETHER_CURL_AUTH_CONFIG_FILE}"
}

labtether_clear_token_environment() {
  # AUTH_TOKEN may remain as a non-exported shell variable for compatibility
  # with code that has not yet been converted, but no curl child may inherit it.
  export -n AUTH_TOKEN 2>/dev/null || true
  unset EXTERNAL_API_TOKEN EXTERNAL_OWNER_TOKEN
  unset LABTETHER_API_TOKEN LABTETHER_OWNER_TOKEN
}

labtether_build_curl_request_args() {
  local url=$1
  local with_auth=${2:-0}
  local sensitive=${3:-$with_auth}
  # --disable must be curl's first argument so an ambient ~/.curlrc cannot
  # inject a proxy, disable verification, or alter authenticated requests.
  LABTETHER_CURL_REQUEST_ARGS=(--disable)

  labtether_validate_tls_options || return 1
  if ! labtether_value_is_true "${LABTETHER_ALLOW_PROXY:-0}"; then
    LABTETHER_CURL_REQUEST_ARGS+=(--noproxy '*')
  fi
  local scheme=""
  scheme=${url%%:*}
  scheme=$(printf '%s' "$scheme" | tr '[:upper:]' '[:lower:]')
  if [[ "$scheme" == "https" || "$scheme" == "wss" ]]; then
    if labtether_value_is_true "${LABTETHER_INSECURE_TLS:-0}"; then
      if [[ "$sensitive" == "1" ]]; then
        log_fail "refusing to send sensitive credentials with TLS verification disabled"
        return 1
      fi
      LABTETHER_CURL_REQUEST_ARGS+=(--insecure)
    elif [[ -n "${LABTETHER_CA_FILE:-}" ]]; then
      LABTETHER_CURL_REQUEST_ARGS+=(--cacert "${LABTETHER_CA_FILE}")
    fi
  fi

  if [[ "$sensitive" == "1" ]]; then
    local normalized_url=""
    normalized_url=$(printf '%s' "$url" | tr '[:upper:]' '[:lower:]')
    case "$scheme" in
      https|wss)
        ;;
      http|ws)
        if [[ ! "$normalized_url" =~ ^(http|ws)://(localhost|127\.0\.0\.1|\[::1\])(:[0-9]+)?(/|$) ]]; then
          log_fail "refusing to send sensitive credentials over non-loopback cleartext transport: $url"
          return 1
        fi
        ;;
      *)
        log_fail "refusing to send sensitive credentials over unsupported URL scheme: $url"
        return 1
        ;;
    esac
  fi
  if [[ "$with_auth" == "1" ]]; then
    if [[ -z "${LABTETHER_CURL_AUTH_CONFIG_FILE}" || ! -r "${LABTETHER_CURL_AUTH_CONFIG_FILE}" ]]; then
      log_fail "secure curl authentication is not initialized"
      return 1
    fi
    LABTETHER_CURL_REQUEST_ARGS+=(--config "${LABTETHER_CURL_AUTH_CONFIG_FILE}")
  fi
}

# Invoke the real curl binary with TLS trust/config environment overrides
# removed. Request policy still comes from labtether_build_curl_request_args.
labtether_curl() (
  unset CURL_CA_BUNDLE SSL_CERT_FILE SSL_CERT_DIR SSLKEYLOGFILE
  unset CURL_HOME NETRC CURL_SSL_BACKEND
  local curl_bin=""
  curl_bin=$(type -P curl 2>/dev/null || true)
  if [[ -z "$curl_bin" ]]; then
    log_fail "required command not found: curl"
    exit 127
  fi
  "$curl_bin" "$@"
)

labtether_prepare_private_output_file() {
  local path=$1
  local parent=""
  parent=$(dirname "$path")
  if [[ ! -d "$parent" || -L "$parent" ]]; then
    log_fail "output parent must be a non-symlink directory: $parent"
    return 1
  fi
  if [[ -e "$path" || -L "$path" ]]; then
    log_fail "refusing to overwrite an existing output path: $path"
    return 1
  fi
  if ! (umask 077; set -o noclobber; : >"$path") 2>/dev/null; then
    log_fail "failed to create private output file without clobbering: $path"
    return 1
  fi
  chmod 600 "$path"
}

labtether_prepare_owned_output_dir() {
  local path=$1
  local label=${2:-output directory}
  if [[ -e "$path" && ( ! -d "$path" || -L "$path" ) ]]; then
    log_fail "$label must be a non-symlink directory: $path"
    return 1
  fi
  if [[ ! -d "$path" ]]; then
    (umask 077; mkdir -p "$path") || return 1
  fi
  local owner_uid=""
  local mode=""
  owner_uid=$(labtether_file_owner_uid "$path" 2>/dev/null || true)
  if stat -f '%Lp' "$path" >/dev/null 2>&1; then
    mode=$(stat -f '%Lp' "$path")
  else
    mode=$(stat -c '%a' "$path" 2>/dev/null || true)
  fi
  if [[ "$owner_uid" != "$(id -u)" || ! "$mode" =~ ^[0-7]{3,4}$ || "${mode: -2:1}" =~ [2367] || "${mode: -1}" =~ [2367] ]]; then
    log_fail "$label must be caller-owned and not group/other writable: $path"
    return 1
  fi
}

labtether_make_private_run_dir() {
  local __result_var=$1
  local root=$2
  local prefix=$3
  if [[ ! "$prefix" =~ ^[A-Za-z0-9._-]+$ ]]; then
    log_fail "run-directory prefix contains unsupported characters"
    return 1
  fi
  if [[ -e "$root" && ( ! -d "$root" || -L "$root" ) ]]; then
    log_fail "output root must be a non-symlink directory: $root"
    return 1
  fi
  if [[ ! -d "$root" ]]; then
    (umask 077; mkdir -p "$root") || return 1
  fi
  local owner_uid=""
  local mode=""
  owner_uid=$(labtether_file_owner_uid "$root" 2>/dev/null || true)
  if stat -f '%Lp' "$root" >/dev/null 2>&1; then
    mode=$(stat -f '%Lp' "$root")
  else
    mode=$(stat -c '%a' "$root" 2>/dev/null || true)
  fi
  if [[ ! "$mode" =~ ^[0-7]{3,4}$ ]]; then
    log_fail "could not validate output-root permissions: $root"
    return 1
  fi
  local padded_mode=$mode
  [[ ${#padded_mode} -eq 3 ]] && padded_mode="0${padded_mode}"
  local sticky_digit=${padded_mode:0:1}
  local group_digit=${padded_mode: -2:1}
  local other_digit=${padded_mode: -1}
  local current_uid=""
  current_uid=$(id -u 2>/dev/null || true)
  if [[ "$owner_uid" == "$current_uid" && ! "$group_digit" =~ [2367] && ! "$other_digit" =~ [2367] ]]; then
    : # A private directory owned by the caller is safe.
  elif [[ "$owner_uid" == "0" && "$sticky_digit" =~ [1357] && "$other_digit" =~ [2367] ]]; then
    : # Root-owned sticky temporary directories (for example /tmp on Linux).
  else
    log_fail "output root must be caller-owned and non-writable by others, or a root-owned sticky temporary directory: $root"
    return 1
  fi
  local created=""
  created=$(mktemp -d "${root%/}/${prefix}.XXXXXX") || return 1
  chmod 700 "$created"
  printf -v "$__result_var" '%s' "$created"
}

labtether_cleanup_curl_security() {
  LABTETHER_CURL_AUTH_CONFIG_FILE=""
  if [[ -n "${LABTETHER_SECURE_CURL_DIR}" && -d "${LABTETHER_SECURE_CURL_DIR}" ]]; then
    rm -rf -- "${LABTETHER_SECURE_CURL_DIR}"
  fi
  LABTETHER_SECURE_CURL_DIR=""
}

log_info() {
  printf '%s\n' "$*"
}

log_pass() {
  local message=$1
  printf 'PASS: %s\n' "$message"
}

log_fail() {
  local message=$1
  printf 'FAIL: %s\n' "$message"
}

log_warn() {
  local message=$1
  printf 'WARN: %s\n' "$message"
}

require_command() {
  local cmd=$1
  if ! command -v "$cmd" >/dev/null 2>&1; then
    log_fail "required command not found: $cmd"
    return 1
  fi
}

has_command() {
  local cmd=$1
  command -v "$cmd" >/dev/null 2>&1
}

check_http_status() {
  local url=$1
  local expected=${2:-200}
  local timeout=${3:-5}
  local status
  local -a curl_args=("-sS" "--connect-timeout" "${timeout}" "--max-time" "${timeout}" "-o" "/dev/null" "-w" "%{http_code}")

  labtether_build_curl_request_args "$url" 0 || return 1
  curl_args=("${LABTETHER_CURL_REQUEST_ARGS[@]}" "${curl_args[@]}")

  status=$(labtether_curl "${curl_args[@]}" "${url}" || true)
  if [[ "${status}" == "${expected}" ]]; then
    return 0
  fi

  return 1
}
