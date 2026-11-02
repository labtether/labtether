// @ts-check
// The public domain's origin client certificate also affects outbound fetch.
// A signed HTTP hop to this same Worker's workers.dev address starts a new
// request without that zone setting. Service bindings inherit the caller's
// setting and cannot provide this boundary.

import { errorResponse, readJsonBody } from "./http.js";

export const EGRESS_PATH = "/_internal/apns";
const MAX_ENVELOPE_BYTES = 16 * 1024;
const MAX_PAYLOAD_BYTES = 5120;
const SIGNATURE_HEADER = "x-labtether-egress-signature";
const SIGNATURE_LIFETIME_MS = 30_000;
const CLOCK_SKEW_MS = 5_000;
const TRANSPORT_ERROR_HEADER = "x-labtether-egress-error";
const APPLE_ORIGINS = new Set(["https://api.push.apple.com", "https://api.sandbox.push.apple.com"]);
const APNS_HEADERS = new Set([
  "authorization", "apns-topic", "apns-push-type", "apns-priority",
  "apns-expiration", "apns-collapse-id", "content-type",
]);
const encoder = new TextEncoder();
/** @type {typeof fetch} */
const defaultFetch = (input, init) => fetch(input, init);

/** @typedef {{ APNS_EGRESS_ORIGIN?: string, APNS_EGRESS_SECRET?: string }} EgressEnv */

/** @param {EgressEnv} env */
export function egressConfigured(env) {
  try {
    const origin = new URL(env.APNS_EGRESS_ORIGIN ?? "");
    return origin.origin === env.APNS_EGRESS_ORIGIN && origin.protocol === "https:" &&
      /^labtether-push-relay\.[a-z0-9-]+\.workers\.dev$/.test(origin.hostname) &&
      !origin.port && !origin.username && !origin.password && origin.pathname === "/" &&
      !origin.search && !origin.hash && /^[a-f0-9]{64}$/i.test(env.APNS_EGRESS_SECRET ?? "");
  } catch {
    return false;
  }
}

/** @param {string} secret */
function signingKey(secret) {
  return crypto.subtle.importKey("raw", encoder.encode(secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign", "verify"]);
}

/** @param {ArrayBuffer} bytes */
function hex(bytes) {
  return Array.from(new Uint8Array(bytes), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

/**
 * @param {string} input
 * @param {RequestInit} init
 * @param {EgressEnv} env
 * @param {typeof fetch} [fetchImpl]
 * @param {number} [nowMs]
 */
export async function forwardPush(input, init, env, fetchImpl = defaultFetch, nowMs = Date.now()) {
  if (!egressConfigured(env)) throw new Error("Push transport is not configured");
  const body = JSON.stringify({
    expires: nowMs + SIGNATURE_LIFETIME_MS,
    url: input,
    method: init.method,
    headers: Object.fromEntries(new Headers(init.headers)),
    body: init.body,
  });
  if (encoder.encode(body).byteLength > MAX_ENVELOPE_BYTES) throw new Error("Push transport body is too large");
  const key = await signingKey(env.APNS_EGRESS_SECRET ?? "");
  const signature = hex(await crypto.subtle.sign({ name: "HMAC" }, key, encoder.encode(body)));
  const response = await fetchImpl(new URL(EGRESS_PATH, env.APNS_EGRESS_ORIGIN).href, {
    method: "POST",
    headers: { "content-type": "application/json", [SIGNATURE_HEADER]: signature },
    body,
    redirect: "manual",
    signal: init.signal,
  });
  if ((response.status >= 300 && response.status < 400) || response.headers.has(TRANSPORT_ERROR_HEADER)) {
    await response.body?.cancel().catch(() => {});
    throw new Error("Push transport connection failed");
  }
  return response;
}

/**
 * @param {unknown} value
 * @param {number} nowMs
 * @returns {value is {expires: number, url: string, method: string, headers: Record<string, string>, body: string}}
 */
function validEnvelope(value, nowMs) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const data = /** @type {Record<string, unknown>} */ (value);
  if (Object.keys(data).sort().join(",") !== "body,expires,headers,method,url" ||
      typeof data.expires !== "number" || !Number.isSafeInteger(data.expires) ||
      data.expires < nowMs || data.expires > nowMs + SIGNATURE_LIFETIME_MS + CLOCK_SKEW_MS ||
      data.method !== "POST" || typeof data.url !== "string" || typeof data.body !== "string" ||
      encoder.encode(data.body).byteLength > MAX_PAYLOAD_BYTES ||
      !data.headers || typeof data.headers !== "object" || Array.isArray(data.headers)) return false;
  try {
    const target = new URL(data.url);
    if (!APPLE_ORIGINS.has(target.origin) || target.search || target.hash || target.username || target.password ||
        !/^\/3\/device\/(?:[a-f0-9]{2}){1,100}$/i.test(target.pathname)) return false;
    return Object.entries(data.headers).every(([name, value]) => APNS_HEADERS.has(name) && typeof value === "string");
  } catch {
    return false;
  }
}

/**
 * @param {Request} request
 * @param {EgressEnv} env
 * @param {typeof fetch} [fetchImpl]
 * @param {number} [nowMs]
 */
export async function handleEgress(request, env, fetchImpl = defaultFetch, nowMs) {
  try {
    return await receivePush(request, env, fetchImpl, nowMs);
  } catch {
    // Native fetch errors can contain the device URL. Never log or return them.
    return errorResponse(502, "apns_unreachable", { [TRANSPORT_ERROR_HEADER]: "unreachable" });
  }
}

/**
 * @param {Request} request
 * @param {EgressEnv} env
 * @param {typeof fetch} fetchImpl
 * @param {number} [nowMs]
 */
async function receivePush(request, env, fetchImpl, nowMs) {
  if (!egressConfigured(env)) return errorResponse(503, "not_configured");
  if (new URL(request.url).origin !== env.APNS_EGRESS_ORIGIN || request.method !== "POST") {
    return errorResponse(404, "not_found");
  }
  const signature = request.headers.get(SIGNATURE_HEADER) ?? "";
  if (!/^[a-f0-9]{64}$/i.test(signature)) return errorResponse(403, "forbidden");
  const parsed = await readJsonBody(request, MAX_ENVELOPE_BYTES);
  if (!parsed.ok) return parsed.response;
  const key = await signingKey(env.APNS_EGRESS_SECRET ?? "");
  const bytes = Uint8Array.from(signature.match(/../g) ?? [], (pair) => Number.parseInt(pair, 16));
  const verified = await crypto.subtle.verify({ name: "HMAC" }, key, bytes, encoder.encode(JSON.stringify(parsed.value)));
  if (!verified) return errorResponse(403, "forbidden");
  if (!validEnvelope(parsed.value, nowMs ?? Date.now())) return errorResponse(400, "invalid_request");
  const response = await fetchImpl(parsed.value.url, {
    method: "POST",
    headers: parsed.value.headers,
    body: parsed.value.body,
    redirect: "manual",
    signal: AbortSignal.timeout(10_000),
  });
  if (response.status >= 300 && response.status < 400) {
    await response.body?.cancel().catch(() => {});
    throw new Error("Apple connection refused a redirect");
  }
  return response;
}
