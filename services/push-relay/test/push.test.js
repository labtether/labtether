// Risks covered: pushes must reach the right APNs host and topic with the
// right headers; dead tokens must be dropped so Hubs stop sending; other APNs
// failures must not delete registrations or leak upstream text; tokens, ids
// and payloads must never reach logs.

import assert from "node:assert/strict";
import { afterEach, beforeEach, test } from "node:test";

import {
  BROKEN_PEM,
  NOW_MS,
  TOKEN,
  apnsError,
  apnsOk,
  call,
  fakeApns,
  fakeLimiter,
  makeCtx,
  makeEnv,
  registerToken,
  request,
  resetCaches,
} from "../test-support/fakes.js";

const ALERT_PAYLOAD = { aps: { alert: { title: "Disk full", body: "nas01 /data at 97%" }, sound: "default" }, alert_id: "al_42" };

beforeEach(() => resetCaches());

function pushRequest(registrationId, overrides = {}) {
  return request("/v1/push", {
    method: "POST",
    body: { registration_id: registrationId, push_type: "alert", priority: 10, payload: ALERT_PAYLOAD, ...overrides },
  });
}

function liveActivityPayload(padBytes) {
  return { aps: { event: "update", timestamp: 1, "content-state": { title: "x".repeat(padBytes) } } };
}

test("alert push reaches the production host with Apple's required headers", async () => {
  const env = await makeEnv();
  const registrationId = await registerToken(env);
  const apns = fakeApns(apnsOk);

  const result = await call(
    pushRequest(registrationId, { expiration: 1_800_000_000, collapse_id: "al_42" }),
    env,
    { apns },
  );

  assert.deepEqual([result.status, result.body], [200, { ok: true, apns_id: "6E1F2B3C-4D5E-6F70-8192-A3B4C5D6E7F8" }]);
  assert.equal(apns.calls.length, 1);
  const [sent] = apns.calls;
  assert.equal(sent.url, `https://api.push.apple.com/3/device/${TOKEN}`);
  assert.equal(sent.init.method, "POST");
  assert.match(sent.headers.get("authorization"), /^bearer [\w-]+\.[\w-]+\.[\w-]+$/);
  assert.equal(sent.headers.get("apns-topic"), "com.labtether.mobile");
  assert.equal(sent.headers.get("apns-push-type"), "alert");
  assert.equal(sent.headers.get("apns-priority"), "10");
  assert.equal(sent.headers.get("apns-expiration"), "1800000000");
  assert.equal(sent.headers.get("apns-collapse-id"), "al_42");
  assert.equal(sent.body, JSON.stringify(ALERT_PAYLOAD));
});

test("sandbox registrations use the sandbox host; optional headers stay unset", async () => {
  const env = await makeEnv();
  const registrationId = await registerToken(env, { environment: "sandbox" });
  const apns = fakeApns(apnsOk);
  const result = await call(
    pushRequest(registrationId, { push_type: "background", priority: 5, payload: { aps: { "content-available": 1 } } }),
    env,
    { apns },
  );
  assert.equal(result.status, 200);
  const [sent] = apns.calls;
  assert.equal(sent.url, `https://api.sandbox.push.apple.com/3/device/${TOKEN}`);
  assert.equal(sent.headers.get("apns-push-type"), "background");
  assert.equal(sent.headers.get("apns-priority"), "5");
  assert.equal(sent.headers.get("apns-expiration"), null);
  assert.equal(sent.headers.get("apns-collapse-id"), null);
});

test("Live Activity pushes use the liveactivity topic and the larger payload cap", async () => {
  const env = await makeEnv();
  const registrationId = await registerToken(env, { kind: "liveactivity" });
  const payload = liveActivityPayload(4600);
  const apns = fakeApns(apnsOk);

  const result = await call(
    pushRequest(registrationId, { push_type: "liveactivity", priority: 10, payload }),
    env,
    { apns },
  );
  assert.equal(result.status, 200);
  assert.equal(apns.calls[0].headers.get("apns-topic"), "com.labtether.mobile.push-type.liveactivity");
  assert.equal(apns.calls[0].headers.get("apns-push-type"), "liveactivity");

  const tooLarge = await call(
    pushRequest(registrationId, { push_type: "liveactivity", priority: 10, payload: liveActivityPayload(5200) }),
    env,
    { apns },
  );
  assert.deepEqual([tooLarge.status, tooLarge.body], [413, { error: "payload_too_large", field: "payload" }]);
  assert.equal(apns.calls.length, 1);
});

test("alert payloads over 4096 bytes are refused before APNs", async () => {
  const env = await makeEnv();
  const registrationId = await registerToken(env);
  const apns = fakeApns(apnsOk);
  const payload = { aps: { alert: { body: "x".repeat(4100) } } };
  const result = await call(pushRequest(registrationId, { payload }), env, { apns });
  assert.equal(result.status, 413);
  assert.equal(apns.calls.length, 0);
});

test("push type must match the registration kind and Apple's priority rules", async () => {
  const env = await makeEnv();
  const alertId = await registerToken(env);
  const activityId = await registerToken(env, { kind: "liveactivity" });
  const apns = fakeApns(apnsOk);

  const wrongKind = await call(
    pushRequest(alertId, { push_type: "liveactivity", payload: liveActivityPayload(10) }),
    env,
    { apns },
  );
  assert.deepEqual([wrongKind.status, wrongKind.body], [400, { error: "push_type_mismatch" }]);
  const wrongKindBack = await call(pushRequest(activityId), env, { apns });
  assert.deepEqual([wrongKindBack.status, wrongKindBack.body], [400, { error: "push_type_mismatch" }]);

  const urgentBackground = await call(
    pushRequest(alertId, { push_type: "background", priority: 10, payload: { aps: { "content-available": 1 } } }),
    env,
    { apns },
  );
  assert.deepEqual(urgentBackground.body, { error: "invalid_request", field: "priority" });

  for (const [overrides, field] of [
    [{ priority: 1 }, "priority"],
    [{ expiration: -1 }, "expiration"],
    [{ expiration: 1.5 }, "expiration"],
    [{ collapse_id: "x".repeat(65) }, "collapse_id"],
    [{ collapse_id: "line\nbreak" }, "collapse_id"],
    [{ payload: { alert: "no aps" } }, "payload"],
    [{ registration_id: "short" }, "registration_id"],
    [{ topic: "com.other.app" }, "body"],
  ]) {
    const result = await call(pushRequest(alertId, overrides), env, { apns });
    assert.deepEqual([result.status, result.body], [400, { error: "invalid_request", field }]);
  }
  assert.equal(apns.calls.length, 0);
});

for (const [status, reason] of [
  [410, "Unregistered"],
  [410, "ExpiredToken"],
  [400, "BadDeviceToken"],
  [400, "DeviceTokenNotForTopic"],
]) {
  test(`APNs ${status} ${reason} deletes the registration and answers 410`, async () => {
    const env = await makeEnv();
    const registrationId = await registerToken(env);
    const result = await call(pushRequest(registrationId), env, { apns: fakeApns(apnsError(status, reason)) });
    assert.deepEqual([result.status, result.body], [410, { error: "unregistered" }]);
    assert.equal(env.REGISTRATIONS.entries.size, 0);

    const after = await call(pushRequest(registrationId), env);
    assert.deepEqual([after.status, after.body], [404, { error: "unknown_registration" }]);
  });
}

test("other APNs failures keep the registration and relay only Apple's reason code", async () => {
  const env = await makeEnv();
  const registrationId = await registerToken(env);

  const throttled = await call(pushRequest(registrationId), env, { apns: fakeApns(apnsError(429, "TooManyRequests")) });
  assert.deepEqual([throttled.status, throttled.body], [502, { error: "apns_error", reason: "TooManyRequests" }]);

  const odd = await call(pushRequest(registrationId), env, {
    apns: fakeApns(() => new Response(`{"reason":"<script>${TOKEN}</script>"}`, { status: 500 })),
  });
  assert.deepEqual([odd.status, odd.body], [502, { error: "apns_error", reason: "Unknown" }]);

  const unreachable = await call(pushRequest(registrationId), env, {
    apns: fakeApns(new TypeError(`fetch failed for https://api.push.apple.com/3/device/${TOKEN}`)),
  });
  assert.deepEqual([unreachable.status, unreachable.body], [502, { error: "apns_unreachable" }]);
  assert.equal(env.REGISTRATIONS.entries.size, 1);
});

test("unknown registrations and missing APNs settings are refused without sending", async (t) => {
  t.mock.method(console, "error", () => {});
  const env = await makeEnv();
  const apns = fakeApns(apnsOk);
  const unknownId = "A".repeat(43);
  const unknown = await call(pushRequest(unknownId), env, { apns });
  assert.deepEqual([unknown.status, unknown.body], [404, { error: "unknown_registration" }]);

  const registrationId = await registerToken(env);
  const reads = env.REGISTRATIONS.reads;
  const unconfigured = await call(pushRequest(registrationId), { ...env, APNS_KEY_ID: "" }, { apns });
  assert.deepEqual([unconfigured.status, unconfigured.body], [503, { error: "not_configured" }]);
  assert.equal(env.REGISTRATIONS.reads, reads);

  const badKey = await call(
    pushRequest(registrationId),
    { ...env, APNS_PRIVATE_KEY: BROKEN_PEM },
    { apns },
  );
  assert.deepEqual([badKey.status, badKey.body], [503, { error: "not_configured" }]);
  assert.equal(apns.calls.length, 0);
});

test("pushes are rate limited per registration, keyed by its hash", async () => {
  const env = await makeEnv({ PUSH_LIMIT: fakeLimiter(2) });
  const registrationId = await registerToken(env);
  const otherId = await registerToken(env);
  const statuses = [];
  for (let attempt = 0; attempt < 3; attempt += 1) {
    statuses.push((await call(pushRequest(registrationId), env)).status);
  }
  statuses.push((await call(pushRequest(otherId), env)).status);
  assert.deepEqual(statuses, [200, 200, 429, 200]);
  for (const key of env.PUSH_LIMIT.keys) {
    assert.match(key, /^push:reg:[0-9a-f]{64}$/);
    assert.ok(!key.includes(registrationId));
  }
});

test("pushes are rate limited per client IP before any registration lookup", async () => {
  const env = await makeEnv({ PUSH_IP_LIMIT: fakeLimiter(2) });
  const send = (ip, id) =>
    call(
      request("/v1/push", {
        method: "POST",
        headers: { "cf-connecting-ip": ip },
        body: { registration_id: id, push_type: "alert", priority: 10, payload: ALERT_PAYLOAD },
      }),
      env,
    );
  const probe = (index) => String(index).padStart(43, "A");
  const statuses = [];
  for (let attempt = 0; attempt < 3; attempt += 1) {
    statuses.push((await send("198.51.100.7", probe(attempt))).status);
  }
  const readsBefore = env.REGISTRATIONS.reads;
  const limited = await send("198.51.100.7", probe(9));
  assert.deepEqual([limited.status, limited.body], [429, { error: "rate_limited" }]);
  assert.equal(limited.headers.get("retry-after"), "60");
  assert.equal(env.REGISTRATIONS.reads, readsBefore, "a limited caller must not cost a KV read");
  statuses.push((await send("198.51.100.8", probe(10))).status);
  assert.deepEqual(statuses, [404, 404, 429, 404]);
  assert.ok(env.PUSH_IP_LIMIT.keys.every((key) => key.startsWith("ip:")));

  const unbound = await call(pushRequest(probe(11)), { ...env, PUSH_IP_LIMIT: undefined });
  assert.deepEqual([unbound.status, unbound.body], [503, { error: "not_configured" }]);
});

test("alert registrations slide their expiry at most once per 30 days", async () => {
  const env = await makeEnv();
  const registrationId = await registerToken(env);
  const writesAfterRegister = env.REGISTRATIONS.writes;

  await call(pushRequest(registrationId), env, { now: NOW_MS + 86_400_000 });
  assert.equal(env.REGISTRATIONS.writes, writesAfterRegister, "no write for a recent registration");

  const later = NOW_MS + 31 * 86_400_000;
  const ctx = makeCtx();
  assert.equal((await call(pushRequest(registrationId), env, { now: later, ctx })).status, 200);
  assert.equal(env.REGISTRATIONS.writes, writesAfterRegister + 1);
  const [entry] = env.REGISTRATIONS.entries.values();
  assert.equal(JSON.parse(entry.value).refreshed_at, Math.floor(later / 1000));
  assert.equal(JSON.parse(entry.value).token, TOKEN);
});

test("tokens, registration ids and payloads never reach the logs", async (t) => {
  const logged = [];
  for (const method of ["log", "info", "warn", "error", "debug"]) {
    t.mock.method(console, method, (...args) => logged.push(args.map(String).join(" ")));
  }
  const env = await makeEnv();
  const registrationId = await registerToken(env);
  await call(pushRequest(registrationId), env);
  await call(pushRequest(registrationId), env, { apns: fakeApns(new Error(`boom ${TOKEN}`)) });
  await call(pushRequest(registrationId), env, { apns: fakeApns(apnsError(500, "InternalServerError")) });
  await call(pushRequest(registrationId), { ...env, APNS_PRIVATE_KEY: BROKEN_PEM });
  await call(pushRequest(registrationId), env, { apns: fakeApns(apnsError(410, "Unregistered")) });

  const output = logged.join("\n");
  for (const secret of [TOKEN, registrationId, "nas01", "Disk full"]) {
    assert.ok(!output.includes(secret), "a secret or payload reached the log");
  }
});

afterEach(() => resetCaches());
