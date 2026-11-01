import { afterEach, describe, expect, it, vi } from "vitest";

import { backendAuthHeadersWithCookie } from "../backend";

afterEach(() => {
  vi.unstubAllEnvs();
});

// Hub audit rows named the console instead of the user for every proxied
// request. The client address may go to the Hub, but X-Forwarded-Proto must
// not: it decides the session cookie's Secure flag.
describe("backendAuthHeadersWithCookie", () => {
  it("forwards only a trusted client address and never the protocol", () => {
    const key = "a".repeat(64);
    vi.stubEnv("LABTETHER_INGRESS_KEY", key);
    const request = (extra: Record<string, string>) => new Request("http://192.0.2.10:3000/api/auth/me", {
      headers: { cookie: "labtether_session=s", origin: "http://192.0.2.10:3000", "x-forwarded-for": "203.0.113.9", ...extra },
    });

    const trusted = backendAuthHeadersWithCookie(request({ "x-labtether-ingress-key": key }));
    expect(trusted["X-Forwarded-For"]).toBe("203.0.113.9");
    expect(trusted["X-Forwarded-Proto"]).toBeUndefined();
    expect(trusted.Cookie).toBe("labtether_session=s");

    const untrusted = backendAuthHeadersWithCookie(request({}));
    expect(untrusted["X-Forwarded-For"]).toBeUndefined();
  });
});
