// @ts-check
// Strict request validation. Validators never echo input back: a failure only
// names the offending field, so error bodies keep a constant shape.

export const MAX_ALERT_PAYLOAD_BYTES = 4096;
export const MAX_LIVEACTIVITY_PAYLOAD_BYTES = 5120;
// 9999-12-31T23:59:59Z, the same upper bound the Hub uses for apns-expiration.
const MAX_EXPIRATION = 253_402_300_799;

// APNs tokens are opaque bytes rendered as hex. Apple does not promise a fixed
// length, so accept 1..100 bytes (the Hub's ActivityKit bound) in lowercase.
const TOKEN_PATTERN = /^(?:[0-9a-f]{2}){1,100}$/;
// 32 random bytes in unpadded base64url.
const REGISTRATION_ID_PATTERN = /^[A-Za-z0-9_-]{43}$/;
// At most 64 printable ASCII bytes (Apple's apns-collapse-id limit), so the
// value is always a legal HTTP header value.
const COLLAPSE_ID_PATTERN = /^[\x20-\x7e]{1,64}$/;

export const KINDS = Object.freeze(["alert", "liveactivity"]);
export const ENVIRONMENTS = Object.freeze(["production", "sandbox"]);
export const PUSH_TYPES = Object.freeze(["alert", "background", "liveactivity"]);

const REGISTRATION_FIELDS = new Set(["token", "kind", "environment"]);
const PUSH_FIELDS = new Set([
  "registration_id",
  "push_type",
  "priority",
  "expiration",
  "collapse_id",
  "payload",
]);

/** Which push types each registration kind may receive. */
const COMPATIBLE_PUSH_TYPES = Object.freeze({
  alert: Object.freeze(["alert", "background"]),
  liveactivity: Object.freeze(["liveactivity"]),
});

/**
 * @typedef {"alert" | "liveactivity"} Kind
 * @typedef {"production" | "sandbox"} Environment
 * @typedef {"alert" | "background" | "liveactivity"} PushType
 * @typedef {{ token: string, kind: Kind, environment: Environment }} RegistrationInput
 * @typedef {{
 *   registration_id: string,
 *   push_type: PushType,
 *   priority: 5 | 10,
 *   expiration?: number,
 *   collapse_id?: string,
 *   payloadJson: string,
 * }} PushInput
 * @typedef {{ ok: false, status: number, error: string, field?: string }} Invalid
 */

/**
 * @param {string} field
 * @returns {Invalid}
 */
function invalid(field) {
  return { ok: false, status: 400, error: "invalid_request", field };
}

/**
 * @param {unknown} value
 * @returns {value is Record<string, unknown>}
 */
export function isPlainObject(value) {
  return (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value) &&
    Object.getPrototypeOf(value) === Object.prototype
  );
}

/**
 * @param {Record<string, unknown>} object
 * @param {Set<string>} allowed
 */
function hasOnlyFields(object, allowed) {
  return Object.keys(object).every((key) => allowed.has(key));
}

/** @param {string} value */
export function utf8Length(value) {
  return new TextEncoder().encode(value).byteLength;
}

/**
 * @param {unknown} value
 * @returns {value is string}
 */
export function isRegistrationId(value) {
  return typeof value === "string" && REGISTRATION_ID_PATTERN.test(value);
}

/**
 * @param {unknown} value
 * @returns {value is string}
 */
export function isDeviceToken(value) {
  return typeof value === "string" && TOKEN_PATTERN.test(value);
}

/**
 * @param {string} kind
 * @param {string} pushType
 */
export function isCompatiblePushType(kind, pushType) {
  const allowed = /** @type {Record<string, readonly string[]>} */ (COMPATIBLE_PUSH_TYPES)[kind];
  return Boolean(allowed && allowed.includes(pushType));
}

/**
 * Validates `POST /v1/registrations`. Hex tokens are accepted in either case
 * and normalized to lowercase.
 *
 * @param {unknown} body
 * @returns {{ ok: true, value: RegistrationInput } | Invalid}
 */
export function validateRegistration(body) {
  if (!isPlainObject(body) || !hasOnlyFields(body, REGISTRATION_FIELDS)) {
    return invalid("body");
  }
  const token = typeof body.token === "string" ? body.token.toLowerCase() : null;
  if (!isDeviceToken(token)) return invalid("token");
  if (typeof body.kind !== "string" || !KINDS.includes(body.kind)) return invalid("kind");
  if (typeof body.environment !== "string" || !ENVIRONMENTS.includes(body.environment)) {
    return invalid("environment");
  }
  return {
    ok: true,
    value: {
      token,
      kind: /** @type {Kind} */ (body.kind),
      environment: /** @type {Environment} */ (body.environment),
    },
  };
}

/**
 * Validates `POST /v1/push`. The payload is serialized once here; that exact
 * string is what the relay sends to APNs, so the size check is exact.
 *
 * @param {unknown} body
 * @returns {{ ok: true, value: PushInput } | Invalid}
 */
export function validatePush(body) {
  if (!isPlainObject(body) || !hasOnlyFields(body, PUSH_FIELDS)) return invalid("body");
  if (!isRegistrationId(body.registration_id)) return invalid("registration_id");
  const pushType = body.push_type;
  if (typeof pushType !== "string" || !PUSH_TYPES.includes(pushType)) return invalid("push_type");
  const priority = body.priority;
  if (priority !== 5 && priority !== 10) return invalid("priority");
  // Apple rejects background pushes sent with priority 10.
  if (pushType === "background" && priority !== 5) return invalid("priority");

  const expiration = body.expiration;
  if (
    expiration !== undefined &&
    (typeof expiration !== "number" ||
      !Number.isSafeInteger(expiration) ||
      expiration < 0 ||
      expiration > MAX_EXPIRATION)
  ) {
    return invalid("expiration");
  }
  const collapseId = body.collapse_id;
  if (
    collapseId !== undefined &&
    (typeof collapseId !== "string" || !COLLAPSE_ID_PATTERN.test(collapseId))
  ) {
    return invalid("collapse_id");
  }

  const payload = body.payload;
  if (!isPlainObject(payload) || !isPlainObject(payload.aps)) return invalid("payload");
  const payloadJson = JSON.stringify(payload);
  const limit =
    pushType === "liveactivity" ? MAX_LIVEACTIVITY_PAYLOAD_BYTES : MAX_ALERT_PAYLOAD_BYTES;
  if (utf8Length(payloadJson) > limit) {
    return { ok: false, status: 413, error: "payload_too_large", field: "payload" };
  }

  /** @type {PushInput} */
  const value = {
    registration_id: body.registration_id,
    push_type: /** @type {PushType} */ (pushType),
    priority,
    payloadJson,
  };
  if (expiration !== undefined) value.expiration = /** @type {number} */ (expiration);
  if (collapseId !== undefined) value.collapse_id = /** @type {string} */ (collapseId);
  return { ok: true, value };
}
