import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { GET } from "../route";

describe("notification history proxy", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ history: [] }), { status: 200 }),
    ));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("marks history responses no-store", async () => {
    const response = await GET(new Request("https://console.example.com/api/notifications/history?limit=20"));

    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetch).toHaveBeenCalledWith(
      "https://api.example.com/notifications/history?limit=20",
      expect.objectContaining({ cache: "no-store" }),
    );
  });
});
