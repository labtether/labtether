// Risks covered: a KV leak must not yield usable registration ids; malformed
// or oversized input must be refused before parsing; registration abuse must
// be rate limited; routing must not expose CORS or unexpected methods.

import assert from "node:assert/strict";
import { beforeEach, test } from "node:test";

import { clientKey } from "../src/index.js";
import { ALERT_TTL_SECONDS, LIVEACTIVITY_TTL_SECONDS } from "../src/registrations.js";
import {
  ORIGIN,
  TOKEN,
  call,
  fakeLimiter,
  makeEnv,
  registerToken,
  request,
  resetCaches,
} from "../test-support/fakes.js";

beforeEach(() => resetCaches());

async function sha256Hex(value) {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value));
  return Buffer.from(digest).toString("hex");
}

test("registration stores only SHA-256(id) and the token, never the id", async () => {
  const env = await makeEnv();
  const registrationId = await registerToken(env, { token: TOKEN.toUpperCase() });

  assert.match(registrationId, /^[A-Za-z0-9_-]{43}$/);
  const keys = [...env.REGISTRATIONS.entries.keys()];
  assert.deepEqual(keys, [`reg:${await sha256Hex(registrationId)}`]);
  assert.ok(!env.REGISTRATIONS.dump().includes(registrationId));

  const entry = env.REGISTRATIONS.entries.get(keys[0]);
  const stored = JSON.parse(entry.value);
  assert.equal(stored.token, TOKEN, "hex tokens are normalized to lowercase");
  assert.equal(stored.kind, "alert");
  assert.equal(stored.environment, "production");
  assert.equal(stored.created_at, Math.floor(Date.UTC(2026, 9, 26) / 1000));
  assert.equal(entry.options.expirationTtl, ALERT_TTL_SECONDS);
});

test("Live Activity registrations expire after one day", async () => {
  const env = await makeEnv();
  await registerToken(env, { kind: "liveactivity", environment: "sandbox" });
  const [entry] = env.REGISTRATIONS.entries.values();
  assert.equal(entry.options.expirationTtl, LIVEACTIVITY_TTL_SECONDS);
});

test("each registration gets a fresh random id", async () => {
  const env = await makeEnv();
  const first = await registerToken(env);
  const second = await registerToken(env);
  assert.notEqual(first, second);
  assert.equal(env.REGISTRATIONS.entries.size, 2);
});

test("invalid registration input is refused with a constant error shape", async () => {
  const env = await makeEnv();
  const cases = [
    [{ token: "abc", kind: "alert", environment: "production" }, "token"],
    [{ token: "zz".repeat(32), kind: "alert", environment: "production" }, "token"],
    [{ token: "ab".repeat(101), kind: "alert", environment: "production" }, "token"],
    [{ token: TOKEN, kind: "voip", environment: "production" }, "kind"],
    [{ token: TOKEN, kind: "alert", environment: "development" }, "environment"],
    [{ token: TOKEN, kind: "alert", environment: "production", extra: 1 }, "body"],
    [[TOKEN], "body"],
  ];
  for (const [body, field] of cases) {
    const result = await call(request("/v1/registrations", { method: "POST", body }), env);
    assert.equal(result.status, 400);
    assert.deepEqual(result.body, { error: "invalid_request", field });
  }
  const broken = await call(request("/v1/registrations", { method: "POST", body: "{nope" }), env);
  assert.deepEqual([broken.status, broken.body], [400, { error: "invalid_json" }]);
  assert.equal(env.REGISTRATIONS.entries.size, 0);
});

test("non-JSON content types are refused before the body is read", async () => {
  const env = await makeEnv();
  for (const contentType of ["text/plain", "application/json; charset=utf-16", ""]) {
    const result = await call(
      new Request(`${ORIGIN}/v1/registrations`, {
        method: "POST",
        headers: contentType ? { "content-type": contentType } : {},
        body: JSON.stringify({ token: TOKEN, kind: "alert", environment: "production" }),
      }),
      env,
    );
    assert.deepEqual([result.status, result.body], [415, { error: "unsupported_media_type" }]);
  }
});

test("bodies over 8 KiB are refused, whether declared or streamed", async () => {
  const env = await makeEnv();
  const oversized = JSON.stringify({ token: TOKEN, pad: "x".repeat(9000) });

  const bytes = new TextEncoder().encode(oversized);
  const declaredRequest = request("/v1/registrations", {
    method: "POST",
    body: oversized,
    headers: { "content-length": String(bytes.length) },
  });
  assert.equal(declaredRequest.headers.get("content-length"), String(bytes.length));
  const declared = await call(declaredRequest, env);
  assert.deepEqual([declared.status, declared.body], [413, { error: "body_too_large" }]);

  const stream = new ReadableStream({
    start(controller) {
      for (let offset = 0; offset < bytes.length; offset += 1024) {
        controller.enqueue(bytes.slice(offset, offset + 1024));
      }
      controller.close();
    },
  });
  const streamed = await call(
    new Request(`${ORIGIN}/v1/registrations`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: stream,
      duplex: "half",
    }),
    env,
  );
  assert.deepEqual([streamed.status, streamed.body], [413, { error: "body_too_large" }]);
  assert.equal(env.REGISTRATIONS.entries.size, 0);
});

test("registration is rate limited per client IP", async () => {
  const env = await makeEnv({ REGISTER_LIMIT: fakeLimiter(1) });
  const send = (ip) =>
    call(
      request("/v1/registrations", {
        method: "POST",
        body: { token: TOKEN, kind: "alert", environment: "production" },
        headers: { "cf-connecting-ip": ip },
      }),
      env,
    );
  assert.equal((await send("198.51.100.7")).status, 201);
  const limited = await send("198.51.100.7");
  assert.deepEqual([limited.status, limited.body], [429, { error: "rate_limited" }]);
  assert.equal(limited.headers.get("retry-after"), "60");
  assert.equal((await send("198.51.100.8")).status, 201);
  assert.deepEqual(env.REGISTER_LIMIT.keys, ["ip:198.51.100.7", "ip:198.51.100.7", "ip:198.51.100.8"]);
});

test("IPv6 clients share one rate-limit key per /64", () => {
  const key = (ip) => clientKey(new Request(ORIGIN, { headers: { "cf-connecting-ip": ip } }));
  assert.equal(key("2001:db8:12:34::1"), "ip6:2001:db8:12:34::/64");
  assert.equal(key("2001:0db8:0012:0034:aaaa:bbbb:cccc:dddd"), "ip6:2001:db8:12:34::/64");
  assert.equal(key("2001:db8::1"), "ip6:2001:db8:0:0::/64");
  assert.notEqual(key("2001:db8:12:35::1"), key("2001:db8:12:34::1"));
});

test("DELETE removes a registration and is idempotent", async () => {
  const env = await makeEnv();
  const registrationId = await registerToken(env);

  const removed = await call(request(`/v1/registrations/${registrationId}`, { method: "DELETE" }), env);
  assert.equal(removed.status, 204);
  assert.equal(env.REGISTRATIONS.entries.size, 0);

  const again = await call(request(`/v1/registrations/${registrationId}`, { method: "DELETE" }), env);
  assert.equal(again.status, 204, "a repeat or unknown id gives the same answer");

  const malformed = await call(request("/v1/registrations/not-an-id", { method: "DELETE" }), env);
  assert.deepEqual([malformed.status, malformed.body], [400, { error: "invalid_request", field: "registration_id" }]);
});

test("routing: health, unknown routes, wrong methods and no CORS", async () => {
  const env = await makeEnv();
  const health = await call(request("/v1/health"), env);
  assert.deepEqual([health.status, health.body], [200, { ok: true }]);
  assert.equal(health.headers.get("cache-control"), "no-store");

  const missing = await call(request("/v1/nope"), env);
  assert.deepEqual([missing.status, missing.body], [404, { error: "not_found" }]);
  assert.equal((await call(request("/v1/health/"), env)).status, 404);

  const wrong = await call(request("/v1/push"), env);
  assert.deepEqual([wrong.status, wrong.body], [405, { error: "method_not_allowed" }]);
  assert.equal(wrong.headers.get("allow"), "POST");

  const preflight = await call(
    request("/v1/registrations", {
      method: "OPTIONS",
      headers: { origin: "https://evil.example", "access-control-request-method": "POST" },
    }),
    env,
  );
  assert.equal(preflight.status, 405);
  for (const [name] of preflight.headers) assert.ok(!name.startsWith("access-control-"));
});
