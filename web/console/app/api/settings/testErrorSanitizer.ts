const REDACTED = "[redacted]";

const SECRET_KV_PATTERN = /(["']?(?:token_secret|password|api_key|secret|authorization|x-api-key|token)["']?\s*[:=]\s*)("[^"]*"|'[^']*'|[^,\s}\]]+)/gi;
const AUTH_HEADER_PATTERN = /(authorization\s*[:=]\s*)([^\s,;]+(?:\s+[^\s,;]+)?)/gi;
const PVE_TOKEN_PATTERN = /(pveapitoken=[^=\s]+)=([^\s,;]+)/gi;
const URL_CREDENTIAL_PATTERN = /(https?:\/\/[^/\s:@]+:)([^@/\s]+)(@)/gi;

function extractErrorMessage(payload: unknown): string {
  if (typeof payload === "string") {
    return payload.trim();
  }
  if (!payload || typeof payload !== "object") {
    return "";
  }
  const record = payload as Record<string, unknown>;
  const error = record.error;
  if (typeof error === "string") {
    return error.trim();
  }
  const message = record.message;
  if (typeof message === "string") {
    return message.trim();
  }
  return "";
}

function sanitizeText(message: string, secrets: string[]): string {
  let sanitized = message
    .replaceAll(SECRET_KV_PATTERN, `$1${REDACTED}`)
    .replaceAll(AUTH_HEADER_PATTERN, `$1${REDACTED}`)
    .replaceAll(PVE_TOKEN_PATTERN, `$1=${REDACTED}`)
    .replaceAll(URL_CREDENTIAL_PATTERN, `$1${REDACTED}$3`);

  for (const secret of secrets) {
    const trimmed = secret.trim();
    if (!trimmed) continue;
    sanitized = sanitized.split(trimmed).join(REDACTED);
  }

  return sanitized.trim();
}

export function sanitizeTestRouteError(
  payload: unknown,
  fallback: string,
  secrets: string[] = [],
): { error: string } {
  const message = extractErrorMessage(payload) || fallback;
  const sanitized = sanitizeText(message, secrets);
  return { error: sanitized || fallback };
}
