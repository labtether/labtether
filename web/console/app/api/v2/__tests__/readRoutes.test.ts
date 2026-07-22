import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { GET as getActions } from "../actions/route";
import { GET as getAssets } from "../assets/route";
import { GET as getOpenAPI } from "../openapi.json/route";

describe("production v2 read routes", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("proxies the API documentation page request", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ openapi: "3.0.3", paths: {} }), { status: 200 }));
    const response = await getOpenAPI();

    expect(response.status).toBe(200);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.example.com/api/v2/openapi.json");
  });

  it("proxies backup asset and paginated saved-action reads with session auth", async () => {
    fetchMock
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: [], meta: { total: 0 } }), { status: 200 }));

    await getAssets(new Request("https://console.example.com/api/v2/assets?limit=25"));
    await getActions(new Request("https://console.example.com/api/v2/actions?limit=100&offset=200"));

    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.example.com/api/v2/assets?limit=25");
    expect(fetchMock.mock.calls[1]?.[0]).toBe("https://api.example.com/api/v2/actions?limit=100&offset=200");
    expect(fetchMock.mock.calls[0]?.[1]?.headers).toEqual({ Cookie: "labtether_session=test" });
    expect(fetchMock.mock.calls[1]?.[1]?.headers).toEqual({ Cookie: "labtether_session=test" });
  });
});
