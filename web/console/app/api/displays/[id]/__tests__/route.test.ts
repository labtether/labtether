import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({
    Cookie: "labtether_session=test",
  })),
  resolvedBackendBaseURLs: vi.fn(async () => ({
    api: "https://api.example.com",
    agent: "https://agent.example.com",
  })),
  upstreamErrorPayload: vi.fn(
    (status: number, payload: { error?: string }, fallback: string) => ({
      error: status >= 500 ? fallback : payload.error || fallback,
    }),
  ),
}));

import { GET } from "../route";

describe("display inventory proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ displays: [{ name: "Display 1" }] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("returns a no-store managed display list", async () => {
    const response = await GET(
      new NextRequest("https://console.example.com/api/displays/asset-1"),
      { params: Promise.resolve({ id: "asset-1" }) },
    );
    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/assets/asset-1/displays",
      expect.objectContaining({ cache: "no-store" }),
    );
  });

  it("rejects malformed IDs and redacts backend failures", async () => {
    const malformed = await GET(
      new NextRequest("https://console.example.com/api/displays/bad%2Fid"),
      { params: Promise.resolve({ id: "bad/id" }) },
    );
    expect(malformed.status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();

    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "/private/agent/path" }), {
        status: 500,
        headers: { "Content-Type": "application/json" },
      }),
    );
    const failed = await GET(
      new NextRequest("https://console.example.com/api/displays/asset-1"),
      { params: Promise.resolve({ id: "asset-1" }) },
    );
    expect(failed.status).toBe(500);
    expect(await failed.json()).toEqual({ error: "failed to query displays" });
  });
});
