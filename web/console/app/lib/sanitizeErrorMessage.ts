const REDACTED = "[redacted]";

const SECRET_KV_PATTERN = /(["']?(?:token_secret|password|api_key|secret|authorization|x-api-key|token)["']?\s*[:=]\s*)("[^"]*"|'[^']*'|[^,\s}\]]+)/gi;
const AUTH_HEADER_PATTERN = /(authorization\s*[:=]\s*)([^\s,;]+(?:\s+[^\s,;]+)?)/gi;
const PVE_TOKEN_PATTERN = /(pveapitoken=[^=\s]+)=([^\s,;]+)/gi;
const URL_CREDENTIAL_PATTERN = /(https?:\/\/[^/\s:@]+:)([^@/\s]+)(@)/gi;
const GENERIC_UPSTREAM_ERRORS = new Set([
  "an internal error occurred.",
  "internal server error",
]);
// The console's own API routes answer "<route> endpoint unavailable" when the hub cannot be reached; that names an
// internal route, so the caller's own message shows instead.
const PROXY_UNAVAILABLE = /\bendpoint unavailable$/i;

export function sanitizeErrorMessage(rawMessage: string, fallback: string, secrets: string[] = []): string {
  const raw = (rawMessage ?? "").trim();
  const base = !raw || GENERIC_UPSTREAM_ERRORS.has(raw.toLowerCase()) || PROXY_UNAVAILABLE.test(raw) ? fallback : raw;
  let sanitized = base
    .replaceAll(SECRET_KV_PATTERN, `$1${REDACTED}`)
    .replaceAll(AUTH_HEADER_PATTERN, `$1${REDACTED}`)
    .replaceAll(PVE_TOKEN_PATTERN, `$1=${REDACTED}`)
    .replaceAll(URL_CREDENTIAL_PATTERN, `$1${REDACTED}$3`);

  for (const secret of secrets) {
    const trimmed = secret.trim();
    if (!trimmed) continue;
    sanitized = sanitized.split(trimmed).join(REDACTED);
  }

  return sanitized.trim() || fallback;
}
