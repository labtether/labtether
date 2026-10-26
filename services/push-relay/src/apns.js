// @ts-check
// APNs provider-token authentication and delivery with plain fetch (HTTP/2 is
// negotiated by the Workers runtime). No payload, token or JWT is ever logged.

import { base64url } from "./encoding.js";

const APNS_HOSTS = Object.freeze({
  production: "https://api.push.apple.com",
  sandbox: "https://api.sandbox.push.apple.com",
});
const LIVEACTIVITY_TOPIC_SUFFIX = ".push-type.liveactivity";
const APNS_TIMEOUT_MS = 10_000;

// Apple refuses provider tokens refreshed more often than every 20 minutes and
// rejects tokens older than 60 minutes. Every isolate derives `iat` from the
// same 50-minute epoch, so all isolates present identical claims, each token
// is used for at most 50 minutes, and a fresh isolate never signs early.
export const JWT_EPOCH_SECONDS = 50 * 60;

const APPLE_ID_PATTERN = /^[A-Za-z0-9]{10}$/;
const TOPIC_PATTERN = /^[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+$/;
const REASON_PATTERN = /^[A-Za-z]{1,64}$/;
const APNS_ID_PATTERN = /^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$/;

/**
 * @typedef {{ teamId: string, keyId: string, topic: string, privateKey: string }} ApnsConfig
 * @typedef {{
 *   APNS_TOPIC?: string,
 *   APNS_TEAM_ID?: string,
 *   APNS_KEY_ID?: string,
 *   APNS_PRIVATE_KEY?: string,
 * }} ApnsEnv
 * @typedef {(input: string, init: RequestInit) => Promise<Response>} FetchLike
 * @typedef {{ outcome: "sent", apnsId: string | null }
 *   | { outcome: "unregistered" }
 *   | { outcome: "rejected", status: number, reason: string }
 *   | { outcome: "unreachable" }
 *   | { outcome: "misconfigured" }} SendResult
 */

/** @type {{ pem: string, key: CryptoKey } | null} */
let signingKeyCache = null;
/** @type {{ key: CryptoKey, identity: string, epoch: number, token: string } | null} */
let jwtCache = null;

/** Clears module-scope caches. Tests use this; the Worker never needs it. */
export function resetApnsCaches() {
  signingKeyCache = null;
  jwtCache = null;
}

/**
 * Returns the APNs configuration, or null when any part is missing or invalid.
 *
 * @param {ApnsEnv} env
 * @returns {ApnsConfig | null}
 */
export function apnsConfig(env) {
  const teamId = (env.APNS_TEAM_ID ?? "").trim();
  const keyId = (env.APNS_KEY_ID ?? "").trim();
  const topic = (env.APNS_TOPIC ?? "").trim();
  const privateKey = env.APNS_PRIVATE_KEY ?? "";
  if (
    !APPLE_ID_PATTERN.test(teamId) ||
    !APPLE_ID_PATTERN.test(keyId) ||
    !TOPIC_PATTERN.test(topic) ||
    topic.endsWith(LIVEACTIVITY_TOPIC_SUFFIX) ||
    privateKey.trim() === ""
  ) {
    return null;
  }
  return { teamId, keyId, topic, privateKey };
}

/**
 * Decodes the .p8 PEM (PKCS#8) to DER. Tolerates a key pasted with literal
 * `\n` escapes instead of line breaks.
 *
 * @param {string} pem
 * @returns {Uint8Array}
 */
export function pemToPkcs8(pem) {
  const body = pem
    .replace(/\\n/g, "\n")
    .replace(/-----(?:BEGIN|END) PRIVATE KEY-----/g, "")
    .replace(/\s+/g, "");
  if (body === "" || !/^[A-Za-z0-9+/]+={0,2}$/.test(body)) {
    throw new Error("APNs private key is not a PKCS#8 PEM");
  }
  const binary = atob(body);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

/**
 * @param {string} pem
 * @returns {Promise<CryptoKey>}
 */
async function signingKey(pem) {
  if (signingKeyCache && signingKeyCache.pem === pem) return signingKeyCache.key;
  const key = await crypto.subtle.importKey(
    "pkcs8",
    pemToPkcs8(pem),
    { name: "ECDSA", namedCurve: "P-256" },
    false,
    ["sign"],
  );
  signingKeyCache = { pem, key };
  return key;
}

/** @param {Record<string, unknown>} value */
function base64urlJson(value) {
  return base64url(new TextEncoder().encode(JSON.stringify(value)));
}

/**
 * Returns the cached ES256 provider token for the current epoch, signing a new
 * one when the epoch, key or identity changes. WebCrypto ECDSA signatures are
 * already the raw 64-byte r||s form that JWS ES256 requires.
 *
 * @param {ApnsConfig} config
 * @param {number} nowMs
 * @returns {Promise<string>}
 */
export async function providerToken(config, nowMs) {
  const key = await signingKey(config.privateKey);
  const epoch = Math.floor(nowMs / 1000 / JWT_EPOCH_SECONDS);
  const identity = `${config.teamId}:${config.keyId}`;
  if (jwtCache && jwtCache.key === key && jwtCache.identity === identity && jwtCache.epoch === epoch) {
    return jwtCache.token;
  }
  const signingInput = `${base64urlJson({ alg: "ES256", kid: config.keyId })}.${base64urlJson({
    iss: config.teamId,
    iat: epoch * JWT_EPOCH_SECONDS,
  })}`;
  const signature = await crypto.subtle.sign(
    { name: "ECDSA", hash: "SHA-256" },
    key,
    new TextEncoder().encode(signingInput),
  );
  const token = `${signingInput}.${base64url(new Uint8Array(signature))}`;
  jwtCache = { key, identity, epoch, token };
  return token;
}

/**
 * APNs proved the token can no longer receive pushes for this topic.
 *
 * @param {number} status
 * @param {string} reason
 */
export function isPermanentTokenRejection(status, reason) {
  if (status === 410) return true;
  return status === 400 && (reason === "BadDeviceToken" || reason === "DeviceTokenNotForTopic");
}

/**
 * Extracts Apple's reason code. Anything that is not a short alphabetic code
 * becomes "Unknown", so no upstream text is ever relayed verbatim.
 *
 * @param {Response} response
 * @returns {Promise<string>}
 */
async function readReason(response) {
  try {
    const text = await response.text();
    if (text.length > 1024) return "Unknown";
    const reason = JSON.parse(text)?.reason;
    return typeof reason === "string" && REASON_PATTERN.test(reason) ? reason : "Unknown";
  } catch {
    return "Unknown";
  }
}

/**
 * Sends one push to APNs for a stored registration.
 *
 * @param {{
 *   config: ApnsConfig,
 *   registration: import("./registrations.js").Registration,
 *   push: import("./validate.js").PushInput,
 *   nowMs: number,
 *   fetchImpl: FetchLike,
 * }} request
 * @returns {Promise<SendResult>}
 */
export async function sendPush({ config, registration, push, nowMs, fetchImpl }) {
  /** @type {string} */
  let jwt;
  try {
    jwt = await providerToken(config, nowMs);
  } catch {
    // Never log the key or the import error detail.
    console.error("push-relay: APNs signing key could not be loaded");
    return { outcome: "misconfigured" };
  }
  const topic =
    push.push_type === "liveactivity" ? config.topic + LIVEACTIVITY_TOPIC_SUFFIX : config.topic;
  /** @type {Record<string, string>} */
  const headers = {
    authorization: `bearer ${jwt}`,
    "apns-topic": topic,
    "apns-push-type": push.push_type,
    "apns-priority": String(push.priority),
    "content-type": "application/json",
  };
  if (push.expiration !== undefined) headers["apns-expiration"] = String(push.expiration);
  if (push.collapse_id !== undefined) headers["apns-collapse-id"] = push.collapse_id;

  const url = `${APNS_HOSTS[registration.environment]}/3/device/${registration.token}`;
  /** @type {Response} */
  let response;
  try {
    response = await fetchImpl(url, {
      method: "POST",
      headers,
      body: push.payloadJson,
      signal: AbortSignal.timeout(APNS_TIMEOUT_MS),
    });
  } catch {
    // The error text can contain the request URL, which holds the token.
    return { outcome: "unreachable" };
  }

  if (response.status === 200) {
    await response.body?.cancel().catch(() => {});
    const apnsId = response.headers.get("apns-id");
    return { outcome: "sent", apnsId: apnsId && APNS_ID_PATTERN.test(apnsId) ? apnsId : null };
  }
  const reason = await readReason(response);
  if (isPermanentTokenRejection(response.status, reason)) return { outcome: "unregistered" };
  return { outcome: "rejected", status: response.status, reason };
}
