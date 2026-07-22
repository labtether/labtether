import { isIP } from "node:net";

const MAX_PROXY_HOPS = 5;

export function trustedProxyHopCount(): number {
  const parsed = Number.parseInt(process.env.LABTETHER_TRUST_PROXY_HOPS ?? "0", 10);
  if (!Number.isInteger(parsed) || parsed < 1 || parsed > MAX_PROXY_HOPS) return 0;
  return parsed;
}

/**
 * Return a rate-limit identity from forwarded headers only when the operator
 * explicitly configured the number of trusted reverse-proxy hops. Direct
 * clients can otherwise spoof these headers.
 */
export function trustedClientIdentity(headers: Headers): string {
  const hops = trustedProxyHopCount();
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
