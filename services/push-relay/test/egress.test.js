// Risks: domain push must keep its API limits and dead-token cleanup; the
// signed connection must not become a public proxy or expose credentials
// through redirects, forged requests, expired signatures, or fetch errors.
import assert from "node:assert/strict";
import { beforeEach, test } from "node:test";
import worker from "../src/worker.js";
import { EGRESS_PATH, forwardPush, handleEgress } from "../src/egress.js";
import { TOKEN, NOW_MS, makeCtx, makeEnv, registerToken, request, resetCaches } from "../test-support/fakes.js";

const transportEnv = {
  APNS_EGRESS_ORIGIN: "https://labtether-push-relay.example.workers.dev",
  APNS_EGRESS_SECRET: "a".repeat(64),
};
const appleURL = `https://api.sandbox.push.apple.com/3/device/${TOKEN}`;
const init = { method: "POST", headers: { authorization: "bearer fixture", "content-type": "application/json" }, body: '{"aps":{}}' };
beforeEach(() => resetCaches());

async function signedRequest(url = appleURL, options = init, now = NOW_MS) {
  let captured;
  await forwardPush(url, options, transportEnv, async (url, init) => {
    captured = new Request(url, init);
    return new Response(null, { status: 200 });
  }, now);
  return captured;
}

test("public push uses a signed hop, preserves caller limits, and deletes the dead registration", async (t) => {
  const env = await makeEnv(transportEnv);
  const ctx = makeCtx();
  const registrationId = await registerToken(env, { environment: "sandbox" });
  const calls = [];
  t.mock.method(globalThis, "fetch", async (url, options) => {
    calls.push({ url, options });
    assert.equal(options.redirect, "manual"); // Workers rejects redirect: error.
    if (url.startsWith(transportEnv.APNS_EGRESS_ORIGIN)) {
      return worker.fetch(new Request(url, options), env, ctx);
    }
    assert.equal(url, appleURL);
    assert.equal(new Headers(options.headers).get("apns-topic"), "com.labtether.mobile");
    return new Response('{"reason":"BadDeviceToken"}', { status: 400 });
  });
  const response = await worker.fetch(request("/v1/push", {
    method: "POST", headers: { "cf-connecting-ip": "192.0.2.10" },
    body: { registration_id: registrationId, push_type: "alert", priority: 10, payload: { aps: { alert: "test" } } },
  }), env, ctx);
  assert.equal(response.status, 410);
  assert.deepEqual(await response.json(), { error: "unregistered" });
  assert.equal(calls.length, 2);
  assert.equal(env.REGISTRATIONS.entries.size, 0);
  assert.deepEqual(env.PUSH_IP_LIMIT.keys, ["ip:192.0.2.10"]);
  assert.equal(env.PUSH_LIMIT.keys.length, 1);
});

test("unsigned, tampered, expired, and wrong-host internal requests never reach Apple", async (t) => {
  let calls = 0;
  const upstream = async () => { calls += 1; return new Response(); };
  const url = transportEnv.APNS_EGRESS_ORIGIN + EGRESS_PATH;
  assert.equal((await handleEgress(new Request(url, { method: "POST" }), transportEnv, upstream, NOW_MS)).status, 403);
  const signed = await signedRequest();
  const tampered = JSON.parse(await signed.clone().text());
  tampered.body = '{"aps":{"alert":"changed"}}';
  assert.equal((await handleEgress(new Request(url, {
    method: "POST", headers: signed.headers, body: JSON.stringify(tampered),
  }), transportEnv, upstream, NOW_MS)).status, 403);
  assert.equal((await handleEgress(await signedRequest(appleURL, init, NOW_MS - 31_000), transportEnv, upstream, NOW_MS)).status, 400);
  assert.equal((await handleEgress(new Request("https://push.example.test" + EGRESS_PATH, signed), transportEnv, upstream, NOW_MS)).status, 404);
  // A caller cannot hold the body open to use a signature after its expiry.
  const delayed = await signedRequest();
  const body = await delayed.text();
  let clock = NOW_MS;
  t.mock.method(Date, "now", () => clock);
  let controller;
  const stream = new ReadableStream({ start(value) { controller = value; } });
  const pending = handleEgress(new Request(url, {
    method: "POST", headers: delayed.headers, body: stream, duplex: "half",
  }), transportEnv, upstream);
  clock += 31_000;
  controller.enqueue(new TextEncoder().encode(body));
  controller.close();
  assert.equal((await pending).status, 400);
  assert.equal(calls, 0);
});

test("even signed envelopes cannot target other hosts or override host headers and payload limits", async () => {
  let calls = 0;
  const upstream = async () => { calls += 1; return new Response(); };
  for (const [url, options] of [
    ["https://example.test/3/device/aa", init],
    ["https://api.sandbox.push.apple.com:2197/3/device/aa", init],
    ["https://user:pass@api.sandbox.push.apple.com/3/device/aa", init],
    [appleURL + "?redirect=1", init],
    [appleURL + "f", init],
    [appleURL, { ...init, method: "DELETE" }],
    [appleURL, { ...init, headers: { ...init.headers, host: "example.test" } }],
    [appleURL, { ...init, body: "x".repeat(5121) }],
  ]) {
    const response = await handleEgress(await signedRequest(url, options), transportEnv, upstream, NOW_MS);
    assert.equal(response.status, 400);
  }
  assert.equal(calls, 0);
  const large = { ...init, body: JSON.stringify({ aps: { alert: "\"".repeat(2400) } }) };
  assert.equal((await handleEgress(await signedRequest(appleURL, large), transportEnv, upstream, NOW_MS)).status, 200);
  assert.equal(calls, 1);
});

test("redirects use the Workers-supported manual mode and transport failures return no private text", async (t) => {
  const logs = [];
  t.mock.method(console, "error", (...args) => logs.push(args));
  const redirect = async (_url, options) => {
    assert.equal(options.redirect, "manual");
    return new Response(null, { status: 307, headers: { location: "https://example.test/" } });
  };
  await assert.rejects(forwardPush(appleURL, init, transportEnv, redirect, NOW_MS), /connection failed/);
  const response = await handleEgress(await signedRequest(), transportEnv, async () => {
    throw new TypeError(`network failed for ${appleURL} with ${transportEnv.APNS_EGRESS_SECRET}`);
  }, NOW_MS);
  assert.equal(response.status, 502);
  assert.deepEqual(await response.json(), { error: "apns_unreachable" });
  assert.deepEqual(logs, []);
  const rejected = await handleEgress(await signedRequest(), transportEnv, redirect, NOW_MS);
  assert.equal(rejected.status, 502);
});

test("missing transport config fails closed while health stays available", async () => {
  const env = await makeEnv();
  const ctx = makeCtx();
  assert.equal((await worker.fetch(request("/v1/health"), env, ctx)).status, 200);
  const response = await worker.fetch(request("/v1/push", { method: "POST", body: {} }), env, ctx);
  assert.equal(response.status, 503);
  assert.deepEqual(await response.json(), { error: "not_configured" });
  const internal = new Request(transportEnv.APNS_EGRESS_ORIGIN + EGRESS_PATH, { method: "POST" });
  assert.equal((await handleEgress(internal, {})).status, 503);
});
