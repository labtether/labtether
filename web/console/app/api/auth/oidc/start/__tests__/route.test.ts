import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://labtether:8443", agent: "https://labtether:8443" })),
  shouldUseSecureWebSocket: vi.fn(() => false),
}));

import { GET } from "../route";

describe("OIDC start", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  // When the Hub refuses the console's address, the login page must say how
  // to allow it instead of "not available right now".
  it("maps the Hub refusing the callback address to its own login error", async () => {
    const fetchMock = vi.fn<(url: string, init?: RequestInit) => Promise<Response>>(async () =>
      Response.json({ error: "redirect_uri origin is not allowed" }, { status: 400 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const start = () => GET(new Request("http://192.0.2.10:3000/api/auth/oidc/start", {
      headers: { host: "192.0.2.10:3000" },
    }));

    expect((await start()).headers.get("location")).toContain("/login?error=sso_address_refused");
    fetchMock.mockImplementationOnce(async () => Response.json({ error: "oidc is not enabled" }, { status: 404 }));
    expect((await start()).headers.get("location")).toContain("/login?error=sso_unavailable");
  });
});
