import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
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

import { DELETE } from "../route";

describe("desktop session teardown proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("deletes the owned backend session through a no-store request", async () => {
    const response = await DELETE(
      new NextRequest(
        "https://console.example.com/api/desktop/session/session-1",
        {
          method: "DELETE",
          headers: { origin: "https://console.example.com" },
        },
      ),
      { params: Promise.resolve({ id: "session-1" }) },
    );

    expect(response.status).toBe(204);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/desktop/sessions/session-1",
      expect.objectContaining({ method: "DELETE", cache: "no-store" }),
    );
  });

  it("rejects cross-origin and malformed teardown attempts before proxying", async () => {
    const crossOrigin = await DELETE(
      new NextRequest(
        "https://console.example.com/api/desktop/session/session-1",
        {
          method: "DELETE",
          headers: { origin: "https://evil.example.com" },
        },
      ),
      { params: Promise.resolve({ id: "session-1" }) },
    );
    expect(crossOrigin.status).toBe(403);

    const malformed = await DELETE(
      new NextRequest(
        "https://console.example.com/api/desktop/session/bad%2Fid",
        {
          method: "DELETE",
          headers: { origin: "https://console.example.com" },
        },
      ),
      { params: Promise.resolve({ id: "bad/id" }) },
    );
    expect(malformed.status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
