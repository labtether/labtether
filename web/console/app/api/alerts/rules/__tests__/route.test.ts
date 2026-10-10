import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com" })),
}));

import { GET } from "../route";

describe("alert rules console proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("forwards an export page with session auth", async () => {
    const page = { rules: [{ id: "rule-101" }] };
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(page), { status: 200 }));

    const response = await GET(new Request("https://console.example.com/api/alerts/rules?limit=100&offset=100"));

    expect(response.status).toBe(200);
    expect(await response.json()).toEqual(page);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.example.com/alerts/rules?limit=100&offset=100");
    expect(fetchMock.mock.calls[0]?.[1]?.headers).toEqual({ Cookie: "labtether_session=test" });
  });
});
