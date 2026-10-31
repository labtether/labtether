import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://hub.example.com", agent: "https://hub.example.com" })),
  shouldUseSecureWebSocket: vi.fn(() => true),
}));

import { GET } from "../route";

// A callback link made in another browser must not sign this browser in.
describe("OIDC callback browser binding", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ next: "/" }), {
        status: 200,
        headers: { "Content-Type": "application/json", "Set-Cookie": "labtether_session=abc; Path=/; HttpOnly" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    vi.clearAllMocks();
  });

  function callback(cookie?: string, extra: Record<string, string> = {}) {
    const headers: Record<string, string> = { host: "console.example.com", ...extra };
    if (cookie) headers.cookie = cookie;
    return GET(new Request("https://console.example.com/api/auth/oidc/callback?code=c1&state=s1", { headers }));
  }

  it("rejects a callback without the starting browser's state cookie", async () => {
    for (const cookie of [undefined, "labtether_oidc_state=other"]) {
      const response = await callback(cookie);
      expect(response.headers.get("location")).toContain("/login?error=");
      expect(response.headers.get("set-cookie") ?? "").not.toContain("labtether_session=abc");
    }
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("finishes sign-in for the browser that started it", async () => {
    const response = await callback("labtether_oidc_state=s1");
    expect(fetchMock).toHaveBeenCalledOnce();
    const cookies = response.headers.getSetCookie();
    expect(cookies.some((value) => value.startsWith("labtether_session=abc"))).toBe(true);
    expect(cookies.some((value) => value.startsWith("labtether_oidc_state=;"))).toBe(true);
  });

  // A Secure session cookie set over plain HTTP is dropped by the browser, so
  // SSO sign-in failed there. The callback has no Origin; the Secure probe
  // cookie from the start route is kept by the browser only on HTTPS.
  it("tells the Hub about plain HTTP only when the browser dropped the Secure probe", async () => {
    const key = "a".repeat(64);
    vi.stubEnv("LABTETHER_INGRESS_KEY", key);
    const keyed = { "x-labtether-ingress-key": key };
    const forwardedProto = () => {
      const init = fetchMock.mock.calls.at(-1)?.[1] as RequestInit;
      return (init.headers as Record<string, string>)["X-Forwarded-Proto"];
    };

    await callback("labtether_oidc_state=s1", keyed);
    expect(forwardedProto()).toBe("http");
    await callback("labtether_oidc_state=s1; labtether_oidc_tls=1", keyed);
    expect(forwardedProto()).toBeUndefined();
    await callback("labtether_oidc_state=s1");
    expect(forwardedProto()).toBeUndefined();
  });
});
