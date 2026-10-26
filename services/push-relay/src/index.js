// @ts-check
// LabTether push relay: lets self-hosted Hubs push to the official iOS app
// without holding Apple's APNs key. See README.md for the API contract.

import { apnsConfig, sendPush } from "./apns.js";
import { errorResponse, json, methodNotAllowed, noContent, readJsonBody } from "./http.js";
import {
  createRegistration,
  deleteRegistration,
  loadRegistration,
  refreshIfStale,
  registrationKey,
} from "./registrations.js";
import {
  isCompatiblePushType,
  isRegistrationId,
  validatePush,
  validateRegistration,
} from "./validate.js";

/**
 * @typedef {{ limit(options: { key: string }): Promise<{ success: boolean }> }} RateLimiter
 * @typedef {import("./apns.js").ApnsEnv & {
 *   REGISTRATIONS: import("./registrations.js").RegistrationStore,
 *   PUSH_LIMIT?: RateLimiter,
 *   PUSH_IP_LIMIT?: RateLimiter,
 *   REGISTER_LIMIT?: RateLimiter,
 * }} Env
 * @typedef {{ waitUntil(promise: Promise<unknown>): void }} ExecutionContextLike
 * @typedef {{ fetch: import("./apns.js").FetchLike, now: () => number }} Deps
 */

const REGISTRATION_PATH = /^\/v1\/registrations\/([^/]+)$/;

/** @type {Deps} */
const defaultDeps = {
  // Keep the global binding: calling a detached `fetch` can throw.
  fetch: (input, init) => fetch(input, init),
  now: () => Date.now(),
};

export default {
  /**
   * @param {Request} request
   * @param {Env} env
   * @param {ExecutionContextLike} ctx
   */
  fetch(request, env, ctx) {
    return handleRequest(request, env, ctx);
  },
};

/**
 * @param {Request} request
 * @param {Env} env
 * @param {ExecutionContextLike} ctx
 * @param {Deps} [deps]
 * @returns {Promise<Response>}
 */
export async function handleRequest(request, env, ctx, deps = defaultDeps) {
  try {
    return await route(request, env, ctx, deps);
  } catch (error) {
    // Log only the error class: messages can carry URLs or request content.
    console.error("push-relay: unhandled error", error instanceof Error ? error.name : typeof error);
    return errorResponse(500, "internal");
  }
}

/**
 * @param {Request} request
 * @param {Env} env
 * @param {ExecutionContextLike} ctx
 * @param {Deps} deps
 */
function route(request, env, ctx, deps) {
  const { pathname } = new URL(request.url);
  const method = request.method;
  if (pathname === "/v1/health") {
    return method === "GET" ? json(200, { ok: true }) : methodNotAllowed(["GET"]);
  }
  if (pathname === "/v1/registrations") {
    return method === "POST" ? register(request, env, deps) : methodNotAllowed(["POST"]);
  }
  const match = REGISTRATION_PATH.exec(pathname);
  if (match) {
    return method === "DELETE" ? unregister(request, env, match[1]) : methodNotAllowed(["DELETE"]);
  }
  if (pathname === "/v1/push") {
    return method === "POST" ? push(request, env, ctx, deps) : methodNotAllowed(["POST"]);
  }
  return errorResponse(404, "not_found");
}

/**
 * Rate-limit key for the caller. IPv6 clients are grouped by /64, because one
 * subscriber usually controls a whole /64 and could otherwise rotate freely.
 *
 * @param {Request} request
 */
export function clientKey(request) {
  const ip = (request.headers.get("cf-connecting-ip") ?? "").trim().toLowerCase();
  if (!ip.includes(":")) return `ip:${ip || "unknown"}`;
  const [head, tail] = ip.split("::");
  const headParts = head ? head.split(":") : [];
  const tailParts = tail ? tail.split(":") : [];
  const groups =
    tail === undefined
      ? headParts
      : [...headParts, ...Array(Math.max(8 - headParts.length - tailParts.length, 0)).fill("0"), ...tailParts];
  const prefix = groups.slice(0, 4).map((group) => group.replace(/^0+(?=.)/, ""));
  return `ip6:${prefix.join(":")}::/64`;
}

/**
 * @param {RateLimiter | undefined} limiter
 * @param {string} key
 * @returns {Promise<Response | null>} a response when the request must stop
 */
async function enforceLimit(limiter, key) {
  // A missing binding is a deployment error; fail closed rather than unlimited.
  if (!limiter || typeof limiter.limit !== "function") return errorResponse(503, "not_configured");
  const { success } = await limiter.limit({ key });
  return success ? null : errorResponse(429, "rate_limited", { "retry-after": "60" });
}

/** @param {import("./validate.js").Invalid} invalid */
function invalidResponse(invalid) {
  return json(invalid.status, { error: invalid.error, field: invalid.field ?? null });
}

/**
 * POST /v1/registrations — called by the iOS app.
 *
 * @param {Request} request
 * @param {Env} env
 * @param {Deps} deps
 */
async function register(request, env, deps) {
  const limited = await enforceLimit(env.REGISTER_LIMIT, clientKey(request));
  if (limited) return limited;
  const body = await readJsonBody(request);
  if (!body.ok) return body.response;
  const parsed = validateRegistration(body.value);
  if (!parsed.ok) return invalidResponse(parsed);
  const registrationId = await createRegistration(
    env.REGISTRATIONS,
    parsed.value,
    Math.floor(deps.now() / 1000),
  );
  return json(201, { registration_id: registrationId });
}

/**
 * DELETE /v1/registrations/:id — idempotent; never reveals whether it existed.
 *
 * @param {Request} request
 * @param {Env} env
 * @param {string} registrationId
 */
async function unregister(request, env, registrationId) {
  const limited = await enforceLimit(env.REGISTER_LIMIT, clientKey(request));
  if (limited) return limited;
  if (!isRegistrationId(registrationId)) {
    return json(400, { error: "invalid_request", field: "registration_id" });
  }
  await deleteRegistration(env.REGISTRATIONS, await registrationKey(registrationId));
  return noContent();
}

/**
 * POST /v1/push — called by a Hub.
 *
 * @param {Request} request
 * @param {Env} env
 * @param {ExecutionContextLike} ctx
 * @param {Deps} deps
 */
async function push(request, env, ctx, deps) {
  // Per caller first: every request with a well-formed id costs a KV read, so
  // probing random ids must be bounded per IP, not only per id.
  const callerLimited = await enforceLimit(env.PUSH_IP_LIMIT, clientKey(request));
  if (callerLimited) return callerLimited;
  const body = await readJsonBody(request);
  if (!body.ok) return body.response;
  const parsed = validatePush(body.value);
  if (!parsed.ok) return invalidResponse(parsed);
  const config = apnsConfig(env);
  if (!config) return errorResponse(503, "not_configured");

  const key = await registrationKey(parsed.value.registration_id);
  // Keyed by the hash, so the bearer id never reaches rate-limit analytics.
  const limited = await enforceLimit(env.PUSH_LIMIT, `push:${key}`);
  if (limited) return limited;
  const registration = await loadRegistration(env.REGISTRATIONS, key);
  if (!registration) return errorResponse(404, "unknown_registration");
  if (!isCompatiblePushType(registration.kind, parsed.value.push_type)) {
    return errorResponse(400, "push_type_mismatch");
  }

  const nowMs = deps.now();
  const result = await sendPush({
    config,
    registration,
    push: parsed.value,
    nowMs,
    fetchImpl: deps.fetch,
  });
  switch (result.outcome) {
    case "sent":
      ctx.waitUntil(
        refreshIfStale(env.REGISTRATIONS, key, registration, Math.floor(nowMs / 1000)).catch(() => {
          console.error("push-relay: registration refresh failed");
        }),
      );
      return json(200, { ok: true, apns_id: result.apnsId });
    case "unregistered":
      await deleteRegistration(env.REGISTRATIONS, key);
      return errorResponse(410, "unregistered");
    case "rejected":
      return json(502, { error: "apns_error", reason: result.reason });
    case "misconfigured":
      return errorResponse(503, "not_configured");
    default:
      return errorResponse(502, "apns_unreachable");
  }
}
