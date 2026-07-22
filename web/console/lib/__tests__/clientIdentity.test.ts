import { afterEach, describe, expect, it, vi } from "vitest";

import { loginRateLimitKey, trustedClientIdentity } from "../clientIdentity";

afterEach(() => {
  vi.unstubAllEnvs();
});

describe("trustedClientIdentity", () => {
  it("does not trust caller-supplied forwarding headers by default", () => {
    const headers = new Headers({ "x-forwarded-for": "203.0.113.10" });
    expect(trustedClientIdentity(headers)).toBe("forwarders-untrusted");
  });

  it("selects the client before the configured trusted proxy chain", () => {
    vi.stubEnv("LABTETHER_TRUST_PROXY_HOPS", "2");
    const headers = new Headers({ "x-forwarded-for": "203.0.113.10, 10.0.0.8" });
    expect(trustedClientIdentity(headers)).toBe("203.0.113.10");
  });

  it("rejects malformed forwarded identities", () => {
    vi.stubEnv("LABTETHER_TRUST_PROXY_HOPS", "1");
    const headers = new Headers({ "x-forwarded-for": "attacker-key" });
    expect(trustedClientIdentity(headers)).toBe("forwarder-invalid");
  });

  it("includes the normalized account in login buckets", () => {
    expect(loginRateLimitKey(" Owner.One ", new Headers())).toBe(
      "login:owner.one:forwarders-untrusted",
    );
  });
});
