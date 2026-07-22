#!/usr/bin/env bash
# Setup Authentik for OIDC testing with LabTether.
# Creates an OAuth2/OIDC application and outputs the env vars needed.
#
# Usage: ./scripts/setup-authentik-test.sh
# Prerequisites: Authentik must be running (docker compose -f deploy/testing/docker-compose.authentik.yml up -d)

set -euo pipefail
set +x
set +a
umask 077

unset AUTHENTIK_TOKEN AUTH_TOKEN

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

for command_name in curl jq mktemp python3 seq; do
    require_command "$command_name" || exit 1
done

AUTHENTIK_URL="http://localhost:9000"
# Bootstrap token set in docker-compose.authentik.yml
AUTHENTIK_TOKEN="labtether-test-bootstrap-token"

# LabTether callback URL (dev mode frontend)
LABTETHER_CALLBACK_URL="http://localhost:3000/api/auth/oidc/callback"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; }
error() { echo -e "${RED}[x]${NC} $*"; exit 1; }

AUTH_TOKEN="$AUTHENTIK_TOKEN"
labtether_prepare_curl_auth "$AUTH_TOKEN" || exit 1
unset AUTHENTIK_TOKEN AUTH_TOKEN
trap labtether_cleanup_curl_security EXIT

# --- Helper: API call ---
api() {
    local method="$1" endpoint="$2"
    shift 2
    local payload=""
    if [[ "${1:-}" == "-d" || "${1:-}" == "--data" ]]; then
        payload="${2:-}"
        shift 2
    fi
    labtether_build_curl_request_args "${AUTHENTIK_URL}/api/v3${endpoint}" 1 || return 1
    local -a args=("${LABTETHER_CURL_REQUEST_ARGS[@]}" -sf --connect-timeout 5 --max-time 30 \
        --max-filesize $((10 * 1024 * 1024)) \
        "${AUTHENTIK_URL}/api/v3${endpoint}" \
        -X "$method" \
        -H "Content-Type: application/json" "$@")
    if [[ -n "$payload" ]]; then
        labtether_curl "${args[@]}" --data-binary @- <<<"$payload"
    else
        labtether_curl "${args[@]}"
    fi
}

# --- Wait for Authentik to be ready ---
info "Waiting for Authentik to be ready..."
for i in $(seq 1 60); do
    labtether_build_curl_request_args "${AUTHENTIK_URL}/-/health/ready/" 0 || exit 1
    if labtether_curl "${LABTETHER_CURL_REQUEST_ARGS[@]}" -sf --max-filesize 1048576 --connect-timeout 2 --max-time 5 "${AUTHENTIK_URL}/-/health/ready/" >/dev/null 2>&1; then
        info "Authentik is ready."
        break
    fi
    if [ "$i" -eq 60 ]; then
        error "Authentik did not become ready within 60 seconds."
    fi
    sleep 2
done

# --- Verify API access ---
info "Verifying API access..."
API_CHECK=$(api GET "/core/users/me/" 2>/dev/null) || error "API authentication failed. Is the bootstrap token correct?"
ADMIN_USER=$(echo "$API_CHECK" | python3 -c "import sys,json; print(json.load(sys.stdin)['user']['username'])" 2>/dev/null)
if [[ -z "$ADMIN_USER" || "$ADMIN_USER" =~ [[:cntrl:]] ]]; then
    error "Authentik returned an invalid administrator username."
fi
info "Authenticated as: ${ADMIN_USER}"

# --- Check if provider already exists ---
EXISTING=$(api GET "/providers/oauth2/?search=labtether-oidc-test" 2>/dev/null || echo '{"results":[]}')
EXISTING_COUNT=$(echo "$EXISTING" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('results',[])))")

if [ "$EXISTING_COUNT" -gt "0" ]; then
    warn "OIDC provider 'labtether-oidc-test' already exists. Fetching existing config..."
    CLIENT_ID=$(echo "$EXISTING" | python3 -c "import sys,json; print(json.load(sys.stdin)['results'][0]['client_id'])")
    CLIENT_SECRET=$(echo "$EXISTING" | python3 -c "import sys,json; print(json.load(sys.stdin)['results'][0]['client_secret'])")
else
    # --- Create a certificate/signing key pair (needed for OIDC) ---
    info "Creating signing key pair..."
    KEYPAIR_RESPONSE=$(api POST "/crypto/certificatekeypairs/generate/" \
        -d '{
            "common_name": "labtether-oidc-test",
            "subject_alt_name": "labtether-oidc-test",
            "validity_days": 365
        }' 2>/dev/null) || true
    KEYPAIR_PK=""
    if echo "$KEYPAIR_RESPONSE" | grep -q '"pk"'; then
        KEYPAIR_PK=$(echo "$KEYPAIR_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['pk'])")
    else
        # Key pair may already exist
        KEYPAIR_PK=$(api GET "/crypto/certificatekeypairs/?search=labtether-oidc-test" | \
            python3 -c "import sys,json; r=json.load(sys.stdin)['results']; print(r[0]['pk'] if r else '')")
    fi

    if [ -z "$KEYPAIR_PK" ]; then
        # Fallback: use any available self-signed cert with a private key
        KEYPAIR_PK=$(api GET "/crypto/certificatekeypairs/?has_key=true&ordering=name" | \
            python3 -c "import sys,json; r=json.load(sys.stdin)['results']; print(r[0]['pk'] if r else '')")
    fi

    # --- Get the default authorization flow ---
    info "Finding default authorization flow..."
    AUTH_FLOW=$(api GET "/flows/instances/?designation=authorization&ordering=slug" | \
        python3 -c "import sys,json; r=json.load(sys.stdin)['results']; print(r[0]['pk'] if r else '')")

    if [ -z "$AUTH_FLOW" ]; then
        error "No authorization flow found in Authentik. The instance may not be fully initialized."
    fi

    # --- Build provider payload ---
    info "Creating OAuth2/OIDC provider..."
    PROVIDER_PAYLOAD=$(jq -cn \
        --arg authorization_flow "$AUTH_FLOW" \
        --arg redirect_uri "$LABTETHER_CALLBACK_URL" \
        --arg signing_key "$KEYPAIR_PK" '
        {
          name: "labtether-oidc-test",
          authorization_flow: $authorization_flow,
          client_type: "confidential",
          redirect_uris: $redirect_uri,
          sub_mode: "hashed_user_id",
          include_claims_in_id_token: true,
          property_mappings: [],
          access_code_validity: "minutes=1",
          access_token_validity: "minutes=5",
          refresh_token_validity: "days=30"
        } + (if $signing_key == "" then {} else {signing_key: $signing_key} end)')

    PROVIDER_RESPONSE=$(api POST "/providers/oauth2/" -d "$PROVIDER_PAYLOAD")
    if ! echo "$PROVIDER_RESPONSE" | grep -q '"pk"'; then
        printf 'Provider creation failed: %s\n' "$(labtether_redact_json_for_log "$PROVIDER_RESPONSE")" >&2
        error "Failed to create OAuth2 provider."
    fi
    PROVIDER_PK=$(echo "$PROVIDER_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['pk'])")
    CLIENT_ID=$(echo "$PROVIDER_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['client_id'])")
    CLIENT_SECRET=$(echo "$PROVIDER_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['client_secret'])")

    if [[ ! "$PROVIDER_PK" =~ ^[0-9]+$ ]]; then
        error "Authentik returned an invalid provider identifier."
    fi

    # --- Create application ---
    info "Creating Authentik application..."
    APP_PAYLOAD=$(jq -cn --argjson provider "$PROVIDER_PK" '{
      name: "LabTether",
      slug: "labtether",
      provider: $provider,
      meta_launch_url: "http://localhost:3000"
    }')
    APP_RESPONSE=$(api POST "/core/applications/" -d "$APP_PAYLOAD" 2>/dev/null) || true

    if echo "$APP_RESPONSE" | grep -q '"pk"'; then
        info "OIDC application created successfully."
    else
        warn "Application creation response: $(labtether_redact_json_for_log "$APP_RESPONSE")"
    fi
fi

# --- Create a test user ---
info "Creating test user..."
TEST_USER_RESPONSE=$(api POST "/core/users/" -d '{
    "username": "testuser",
    "name": "Test User",
    "email": "test@localhost",
    "is_active": true
}' 2>/dev/null) || true

if echo "$TEST_USER_RESPONSE" | grep -q '"pk"'; then
    TEST_USER_PK=$(echo "$TEST_USER_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['pk'])")
    if [[ ! "$TEST_USER_PK" =~ ^[0-9]+$ ]]; then
        error "Authentik returned an invalid test-user identifier."
    fi
    # Set password
    api POST "/core/users/${TEST_USER_PK}/set_password/" -d '{"password": "testpass123!"}' >/dev/null 2>&1 || true
    info "Test user created (credentials remain in this testing-only setup script)."
else
    warn "Test user 'testuser' may already exist (that's fine)."
fi

# --- Output results ---
ISSUER_URL="${AUTHENTIK_URL}/application/o/labtether/"
OIDC_ENV_FILE="$(mktemp "${TMPDIR:-/tmp}/labtether-authentik-oidc.env.XXXXXX")"
{
    printf 'export LABTETHER_OIDC_ISSUER_URL=%q\n' "$ISSUER_URL"
    printf 'export LABTETHER_OIDC_CLIENT_ID=%q\n' "$CLIENT_ID"
    printf 'export LABTETHER_OIDC_CLIENT_SECRET=%q\n' "$CLIENT_SECRET"
    printf 'export LABTETHER_OIDC_DISPLAY_NAME=%q\n' 'Authentik'
} >"$OIDC_ENV_FILE"
chmod 600 "$OIDC_ENV_FILE"
unset CLIENT_SECRET

echo ""
echo "============================================================"
echo "  OIDC Test Setup Complete"
echo "============================================================"
echo ""
echo "Authentik UI:     ${AUTHENTIK_URL}"
echo "Testing account credentials remain in the local testing compose/script files."
echo ""
echo "OIDC environment written to a mode-0600 file: ${OIDC_ENV_FILE}"
printf 'Load it with: source %q\n' "$OIDC_ENV_FILE"
echo ""
echo "Then restart the LabTether backend to pick up the OIDC config."
echo "============================================================"
