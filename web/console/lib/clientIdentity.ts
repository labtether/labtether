import { timingSafeEqual } from "node:crypto";
import { isIP } from "node:net";

const MAX_PROXY_HOPS = 5;
const INGRESS_KEY_HEADER = "x-labtether-ingress-key";

export function trustedProxyHopCount(): number {
  const parsed = Number.parseInt(process.env.LABTETHER_TRUST_PROXY_HOPS ?? "0", 10);
  if (!Number.isInteger(parsed) || parsed < 1 || parsed > MAX_PROXY_HOPS) return 0;
  return parsed;
}

/**
 * The bundled console ingress appends the address it accepted to
 * X-Forwarded-For and sends the console runtime's per-boot key. Only a
 * matching key counts that entry as a real hop; a client that reaches the
 * console directly cannot know the key.
 */
function bundledIngressHops(headers: Headers): number {
  const expected = process.env.LABTETHER_INGRESS_KEY?.trim() ?? "";
  const provided = headers.get(INGRESS_KEY_HEADER)?.trim() ?? "";
  if (!expected || !provided) return 0;
  const expectedBytes = Buffer.from(expected);
  const providedBytes = Buffer.from(provided);
  return expectedBytes.length === providedBytes.length && timingSafeEqual(expectedBytes, providedBytes) ? 1 : 0;
}

/**
 * Return a rate-limit identity from forwarded headers only for hops that are
 * trusted: the bundled ingress (proven by its key) plus any reverse-proxy hops
 * the operator configured in front of it. Direct clients can otherwise spoof
 * these headers.
 */
export function trustedClientIdentity(headers: Headers): string {
  const hops = Math.min(MAX_PROXY_HOPS, trustedProxyHopCount() + bundledIngressHops(headers));
  if (hops === 0) return "forwarders-untrusted";

  const chain = (headers.get("x-forwarded-for") ?? "")
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean)
    .slice(-16);
  const candidate = chain[chain.length - hops]
    ?? (hops === 1 ? headers.get("x-real-ip")?.trim() : undefined)
    ?? "";
  return isIP(candidate) === 0 ? "forwarder-invalid" : candidate.toLowerCase();
}

export function loginRateLimitKey(username: string, headers: Headers): string {
  const account = username.trim().toLowerCase().slice(0, 128) || "invalid-account";
  return `login:${account}:${trustedClientIdentity(headers)}`;
}

/**
 * Reports plain HTTP only from trusted signals: the request came through the
 * bundled ingress (proven by its key, and the ingress serves plain HTTP), the
 * operator has not put another proxy in front of it, and the page that sent
 * the request was itself loaded over http. Anything else, including a request
 * with no Origin, counts as HTTPS so the session cookie stays Secure.
 */
export function browserUsedPlainHTTP(headers: Headers): boolean {
  if (bundledIngressHops(headers) !== 1 || trustedProxyHopCount() !== 0) return false;
  try {
    return new URL(headers.get("origin")?.trim() ?? "").protocol === "http:";
  } catch {
    return false;
  }
}

/**
 * Headers that tell the Hub which client the browser was and, only when it is
 * certain, that the browser used plain HTTP. The Hub trusts them only from
 * this console and keeps the session cookie Secure unless told "http".
 */
export function hubClientForwardingHeaders(request: Request): Record<string, string> {
  const headers: Record<string, string> = {};
  if (browserUsedPlainHTTP(request.headers)) headers["X-Forwarded-Proto"] = "http";
  const client = trustedClientIdentity(request.headers);
  if (isIP(client) !== 0) headers["X-Forwarded-For"] = client;
  return headers;
}
