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

import { POST } from "../route";

describe("recording stop proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ stopped: true }), {
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

  it("stops an owned session through a bounded no-store route", async () => {
    const response = await POST(
      new NextRequest(
        "https://console.example.com/api/recordings/session-1",
        {
          method: "POST",
          headers: { origin: "https://console.example.com" },
        },
      ),
      { params: Promise.resolve({ sessionId: "session-1" }) },
    );
    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/recordings/session-1",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("rejects malformed identifiers and cross-origin mutation", async () => {
    const malformed = await POST(
      new NextRequest("https://console.example.com/api/recordings/bad%2Fid", {
        method: "POST",
        headers: { origin: "https://console.example.com" },
      }),
      { params: Promise.resolve({ sessionId: "bad/id" }) },
    );
    expect(malformed.status).toBe(400);

    const crossOrigin = await POST(
      new NextRequest(
        "https://console.example.com/api/recordings/session-1",
        {
          method: "POST",
          headers: { origin: "https://evil.example.com" },
        },
      ),
      { params: Promise.resolve({ sessionId: "session-1" }) },
    );
    expect(crossOrigin.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
