// @ts-check
// Registration storage in Workers KV.
//
// The registration id is a 256-bit bearer secret held only by the iOS app and
// the Hubs it hands the id to. KV stores SHA-256(id), never the id itself, so a
// KV dump does not yield ids that can push. The APNs token is stored because
// the relay must address APNs; no payload is ever stored.

import { base64url, hex } from "./encoding.js";
import { ENVIRONMENTS, KINDS, isDeviceToken } from "./validate.js";

const KEY_PREFIX = "reg:";
const DAY_SECONDS = 86_400;

// ActivityKit push tokens die with their activity, which stays on screen for
// at most about 12 hours. One day bounds stale rows without cutting one short.
export const LIVEACTIVITY_TTL_SECONDS = DAY_SECONDS;
// Alert registrations slide: a successful push extends them (see below), so
// only registrations that no Hub has used for 400 days expire. This also
// bounds storage for registrations created and never used.
export const ALERT_TTL_SECONDS = 400 * DAY_SECONDS;
// Extend an alert registration at most once per 30 days to keep KV writes rare.
export const ALERT_REFRESH_AFTER_SECONDS = 30 * DAY_SECONDS;

/**
 * @typedef {import("./validate.js").Kind} Kind
 * @typedef {import("./validate.js").Environment} Environment
 * @typedef {{
 *   token: string,
 *   kind: Kind,
 *   environment: Environment,
 *   created_at: number,
 *   refreshed_at: number,
 * }} Registration
 * @typedef {{
 *   get(key: string, options: { type: "json" }): Promise<unknown>,
 *   put(key: string, value: string, options?: { expirationTtl?: number }): Promise<void>,
 *   delete(key: string): Promise<void>,
 * }} RegistrationStore
 */

/** @returns {string} 32 random bytes as unpadded base64url (43 characters). */
export function newRegistrationId() {
  return base64url(crypto.getRandomValues(new Uint8Array(32)));
}

/**
 * KV key for a registration id: `reg:` + hex(SHA-256(id)).
 *
 * @param {string} registrationId
 */
export async function registrationKey(registrationId) {
  const digest = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(registrationId),
  );
  return KEY_PREFIX + hex(digest);
}

/** @param {Kind} kind */
function ttlFor(kind) {
  return kind === "liveactivity" ? LIVEACTIVITY_TTL_SECONDS : ALERT_TTL_SECONDS;
}

/**
 * @param {RegistrationStore} kv
 * @param {string} key
 * @param {Registration} record
 */
function writeRecord(kv, key, record) {
  return kv.put(key, JSON.stringify(record), { expirationTtl: ttlFor(record.kind) });
}

/**
 * @param {RegistrationStore} kv
 * @param {import("./validate.js").RegistrationInput} input
 * @param {number} nowSeconds
 * @returns {Promise<string>} the new registration id
 */
export async function createRegistration(kv, input, nowSeconds) {
  const registrationId = newRegistrationId();
  const key = await registrationKey(registrationId);
  await writeRecord(kv, key, {
    token: input.token,
    kind: input.kind,
    environment: input.environment,
    created_at: nowSeconds,
    refreshed_at: nowSeconds,
  });
  return registrationId;
}

/**
 * Loads a registration. A record that does not have the expected shape is
 * treated as missing rather than being sent to APNs.
 *
 * @param {RegistrationStore} kv
 * @param {string} key
 * @returns {Promise<Registration | null>}
 */
export async function loadRegistration(kv, key) {
  const record = /** @type {Record<string, unknown> | null} */ (await kv.get(key, { type: "json" }));
  if (
    !record ||
    typeof record !== "object" ||
    !isDeviceToken(record.token) ||
    typeof record.kind !== "string" ||
    !KINDS.includes(record.kind) ||
    typeof record.environment !== "string" ||
    !ENVIRONMENTS.includes(record.environment) ||
    !Number.isSafeInteger(record.created_at)
  ) {
    return null;
  }
  const createdAt = /** @type {number} */ (record.created_at);
  const refreshedAt = Number.isSafeInteger(record.refreshed_at)
    ? /** @type {number} */ (record.refreshed_at)
    : createdAt;
  return {
    token: record.token,
    kind: /** @type {Kind} */ (record.kind),
    environment: /** @type {Environment} */ (record.environment),
    created_at: createdAt,
    refreshed_at: refreshedAt,
  };
}

/**
 * @param {RegistrationStore} kv
 * @param {string} key
 */
export function deleteRegistration(kv, key) {
  return kv.delete(key);
}

/**
 * Extends an alert registration's TTL after a successful push, at most once
 * per ALERT_REFRESH_AFTER_SECONDS. Live Activity registrations never slide.
 *
 * @param {RegistrationStore} kv
 * @param {string} key
 * @param {Registration} record
 * @param {number} nowSeconds
 * @returns {Promise<boolean>} whether a refresh was written
 */
export async function refreshIfStale(kv, key, record, nowSeconds) {
  if (record.kind !== "alert") return false;
  if (nowSeconds - record.refreshed_at < ALERT_REFRESH_AFTER_SECONDS) return false;
  await writeRecord(kv, key, { ...record, refreshed_at: nowSeconds });
  return true;
}
