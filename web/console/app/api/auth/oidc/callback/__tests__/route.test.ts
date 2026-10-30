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
    vi.clearAllMocks();
  });

  function callback(cookie?: string) {
    const headers: Record<string, string> = { host: "console.example.com" };
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
});
