// Risk covered: APNs rejects every push if the provider token is malformed,
// signed in the wrong format, too old, or refreshed too often.

import assert from "node:assert/strict";
import { beforeEach, test } from "node:test";

import {
  JWT_EPOCH_SECONDS,
  apnsConfig,
  pemToPkcs8,
  providerToken,
  resetApnsCaches,
} from "../src/apns.js";
import { NOW_MS, makeEnv, pemBlock, testSigningKey } from "../test-support/fakes.js";

beforeEach(() => resetApnsCaches());

function decodeSegment(segment) {
  const base64 = segment.replace(/-/g, "+").replace(/_/g, "/");
  return Uint8Array.from(atob(base64), (char) => char.charCodeAt(0));
}

function decodeJson(segment) {
  return JSON.parse(new TextDecoder().decode(decodeSegment(segment)));
}

test("provider token is an ES256 JWT that verifies with the .p8 public key", async () => {
  const env = await makeEnv();
  const { publicKey } = await testSigningKey();
  const token = await providerToken(apnsConfig(env), NOW_MS);

  const [header, claims, signature] = token.split(".");
  assert.deepEqual(decodeJson(header), { alg: "ES256", kid: "KEY1234567" });
  const payload = decodeJson(claims);
  assert.equal(payload.iss, "TEAM123456");
  assert.deepEqual(Object.keys(payload).sort(), ["iat", "iss"]);

  // JWS ES256 needs the raw 64-byte r||s form, not DER.
  const rawSignature = decodeSegment(signature);
  assert.equal(rawSignature.byteLength, 64);
  const valid = await crypto.subtle.verify(
    { name: "ECDSA", hash: "SHA-256" },
    publicKey,
    rawSignature,
    new TextEncoder().encode(`${header}.${claims}`),
  );
  assert.equal(valid, true);
});

test("provider token is reused within a 50-minute epoch and never older than that", async () => {
  const config = apnsConfig(await makeEnv());
  const epochStartMs = Math.floor(NOW_MS / 1000 / JWT_EPOCH_SECONDS) * JWT_EPOCH_SECONDS * 1000;

  const first = await providerToken(config, epochStartMs + 1_000);
  const sameEpoch = await providerToken(config, epochStartMs + 49 * 60_000);
  assert.equal(sameEpoch, first, "a refresh inside 20 minutes would trip TooManyProviderTokenUpdates");

  const nextEpochMs = epochStartMs + JWT_EPOCH_SECONDS * 1000;
  const next = await providerToken(config, nextEpochMs + 5_000);
  assert.notEqual(next, first);

  const firstIat = decodeJson(first.split(".")[1]).iat;
  const nextIat = decodeJson(next.split(".")[1]).iat;
  assert.equal(nextIat - firstIat, JWT_EPOCH_SECONDS);
  // Used until the next epoch, so its age stays below Apple's 60-minute cap.
  assert.ok(nextEpochMs / 1000 - firstIat <= JWT_EPOCH_SECONDS);
  assert.ok(firstIat <= (epochStartMs + 1_000) / 1000);
});

test("a key pasted with literal \\n escapes decodes to the same PKCS#8 bytes", async () => {
  const { pem } = await testSigningKey();
  const escaped = pem.replace(/\n/g, "\\n");
  assert.deepEqual(pemToPkcs8(escaped), pemToPkcs8(pem));
  // A SEC1 "EC PRIVATE KEY" is not the .p8 PKCS#8 form Apple issues.
  assert.throws(() => pemToPkcs8(pemBlock("EC PRIVATE KEY", "MHcCAQEE")));
});

test("APNs configuration is refused when any setting is missing or malformed", async () => {
  const env = await makeEnv();
  assert.ok(apnsConfig(env));
  assert.equal(apnsConfig({ ...env, APNS_KEY_ID: "" }), null);
  assert.equal(apnsConfig({ ...env, APNS_TEAM_ID: "short" }), null);
  assert.equal(apnsConfig({ ...env, APNS_PRIVATE_KEY: "" }), null);
  assert.equal(apnsConfig({ ...env, APNS_TOPIC: "com.labtether.mobile.push-type.liveactivity" }), null);
});
