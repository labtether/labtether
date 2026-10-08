#!/usr/bin/env bash
# Private files, literal environment parsing, and secret-file reads.

labtether_file_owner_uid() {
  local path=$1
  if stat -f '%u' "$path" >/dev/null 2>&1; then
    stat -f '%u' "$path"
  elif stat -c '%u' "$path" >/dev/null 2>&1; then
    stat -c '%u' "$path"
  else
    return 1
  fi
}

labtether_file_mode() {
  local path=$1
  if stat -f '%Lp' "$path" >/dev/null 2>&1; then
    stat -f '%Lp' "$path"
  elif stat -c '%a' "$path" >/dev/null 2>&1; then
    stat -c '%a' "$path"
  else
    return 1
  fi
}

labtether_file_size() {
  local path=$1
  if stat -f '%z' "$path" >/dev/null 2>&1; then
    stat -f '%z' "$path"
  elif stat -c '%s' "$path" >/dev/null 2>&1; then
    stat -c '%s' "$path"
  else
    return 1
  fi
}

labtether_require_private_file() {
  local path=$1
  local label=${2:-secret file}
  local allow_insecure=${3:-0}
  if [[ ! -f "$path" || ! -r "$path" || -L "$path" ]]; then
    log_fail "$label must be a readable, non-symlink regular file: $path"
    return 1
  fi
  local owner_uid=""
  local current_uid=""
  owner_uid=$(labtether_file_owner_uid "$path" 2>/dev/null || true)
  current_uid=$(id -u 2>/dev/null || true)
  if [[ -z "$owner_uid" || -z "$current_uid" || "$owner_uid" != "$current_uid" ]]; then
    log_fail "$label must be owned by the current user: $path"
    return 1
  fi
  local mode=""
  if stat -f '%Lp' "$path" >/dev/null 2>&1; then
    mode=$(stat -f '%Lp' "$path")
  elif stat -c '%a' "$path" >/dev/null 2>&1; then
    mode=$(stat -c '%a' "$path")
  fi
  if [[ -z "$mode" ]]; then
    log_fail "could not determine permissions for $label: $path"
    return 1
  fi
  if [[ ! "$mode" =~ ^[0-7]{3,4}$ ]]; then
    log_fail "could not validate permissions for $label (reported mode $mode): $path"
    return 1
  fi
  if [[ "$mode" =~ [0-7][0-7]$ && "${mode: -2}" != "00" ]]; then
    if labtether_value_is_true "$allow_insecure"; then
      log_warn "$label is group/other-accessible (mode $mode): $path"
      return 0
    fi
    log_fail "$label must not be group/other-accessible (mode $mode): $path"
    log_fail "fix with: chmod 600 '$path'"
    return 1
  fi
}

# Read one allowlisted value from a dotenv file without executing the file as
# shell code. This deliberately supports only literal dotenv assignments; it
# does not perform command substitution or variable expansion.
labtether_read_env_value() {
  local __value_var=$1
  local path=$2
  local key=$3
  if [[ ! "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    log_fail "invalid dotenv key: $key"
    return 1
  fi

  local line=""
  local trimmed=""
  local raw=""
  local value=""
  local found=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line=${line%$'\r'}
    trimmed=${line#"${line%%[![:space:]]*}"}
    [[ -z "$trimmed" || "$trimmed" == \#* ]] && continue
    if [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?${key}[[:space:]]*=(.*)$ ]]; then
      raw=${BASH_REMATCH[2]}
      raw=${raw#"${raw%%[![:space:]]*}"}
      raw=${raw%"${raw##*[![:space:]]}"}
      if [[ "$raw" == \"* ]]; then
        if [[ ${#raw} -lt 2 || "${raw: -1}" != '"' ]]; then
          log_fail "unterminated double-quoted dotenv value for $key"
          return 1
        fi
        value=${raw:1:${#raw}-2}
        value=${value//\\\"/\"}
        value=${value//\\\\/\\}
      elif [[ "$raw" == \'* ]]; then
        if [[ ${#raw} -lt 2 || "${raw: -1}" != "'" ]]; then
          log_fail "unterminated single-quoted dotenv value for $key"
          return 1
        fi
        value=${raw:1:${#raw}-2}
      else
        # Compose treats a whitespace-delimited # as an inline comment.
        value=${raw%%[[:space:]]\#*}
        value=${value%"${value##*[![:space:]]}"}
      fi
      found=1
    fi
  done <"$path"

  if [[ "$found" == "1" ]]; then
    printf -v "$__value_var" '%s' "$value"
  else
    printf -v "$__value_var" '%s' ''
  fi
}

# Export only literal dotenv assignments whose names match a caller-supplied
# allowlist. The file is never sourced or evaluated as shell code.
labtether_load_env_file_literals() {
  local path=$1
  local allowed_name_pattern=$2
  labtether_require_private_env_file "$path" || return 1

  local line=""
  local key=""
  local loaded_value=""
  local -a allowed_keys=()
  while IFS= read -r line || [[ -n "$line" ]]; do
    line=${line%$'\r'}
    if [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*= ]]; then
      key=${BASH_REMATCH[2]}
      [[ "$key" =~ $allowed_name_pattern ]] || continue
      allowed_keys+=("$key")
    fi
  done <"$path"
  for key in "${allowed_keys[@]}"; do
    labtether_read_env_value loaded_value "$path" "$key" || return 1
    export "$key=$loaded_value"
  done
}

labtether_require_private_env_file() {
  local path=$1
  if ! labtether_require_private_file "$path" "secret env file" "${LABTETHER_ALLOW_INSECURE_ENV_FILE:-0}"; then
    if ! labtether_value_is_true "${LABTETHER_ALLOW_INSECURE_ENV_FILE:-0}"; then
      log_fail "LABTETHER_ALLOW_INSECURE_ENV_FILE=1 is available only for explicit local diagnostics"
    fi
    return 1
  fi
}

labtether_lock_down_private_file() {
  local path=$1
  local label=${2:-private file}
  if [[ ! -f "$path" || -L "$path" ]]; then
    log_fail "$label must be a non-symlink regular file: $path"
    return 1
  fi
  local owner_uid=""
  owner_uid=$(labtether_file_owner_uid "$path" 2>/dev/null || true)
  if [[ -z "$owner_uid" || "$owner_uid" != "$(id -u)" ]]; then
    log_fail "$label must be owned by the current user: $path"
    return 1
  fi
  chmod 600 "$path" || return 1
  labtether_require_private_file "$path" "$label" 0
}

labtether_create_private_file_from_template() {
  local template=$1
  local destination=$2
  local label=${3:-private file}
  if [[ ! -f "$template" || -L "$template" ]]; then
    log_fail "template must be a non-symlink regular file: $template"
    return 1
  fi
  if [[ -e "$destination" || -L "$destination" ]]; then
    log_fail "refusing to overwrite existing $label: $destination"
    return 1
  fi
  if ! (umask 077; set -o noclobber; : >"$destination") 2>/dev/null; then
    log_fail "failed to create $label without clobbering: $destination"
    return 1
  fi
  if ! cp "$template" "$destination"; then
    rm -f -- "$destination"
    return 1
  fi
  chmod 600 "$destination"
  labtether_require_private_file "$destination" "$label" 0
}

labtether_read_private_secret_file() {
  local __value_var=$1
  local path=$2
  local label=${3:-secret file}
  labtether_require_private_file "$path" "$label" 0 || return 1

  local value
  value=$(<"$path")
  if [[ -z "$value" ]]; then
    log_fail "$label is empty: $path"
    return 1
  fi
  case "$value" in
    *$'\n'*|*$'\r'*)
      log_fail "$label must contain exactly one line: $path"
      return 1
      ;;
  esac
  printf -v "$__value_var" '%s' "$value"
}
