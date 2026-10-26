import { NextRequest } from "next/server";
import { afterEach, describe, expect, it, vi } from "vitest";

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
      error: status >= 500 ? fallback : payload?.error || fallback,
    }),
  ),
}));

import { GET } from "../route";

function download(id: string) {
  return GET(
    new NextRequest(`https://console.example.com/api/recordings/${encodeURIComponent(id)}/download`),
    { params: Promise.resolve({ sessionId: id }) },
  );
}

// Operators rely on this proxy to hand the Hub's attachment to the browser
// unchanged, and to surface the Hub's refusal instead of a broken file.
describe("recording download proxy", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("streams the recording as an uncached attachment", async () => {
    const fetchMock = vi.fn(async () =>
      new Response("recording-bytes", {
        status: 200,
        headers: {
          "Content-Type": "application/octet-stream",
          "Content-Disposition": 'attachment; filename="rec-1.bin"',
        },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await download("rec-1");

    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/recordings/rec-1/download",
      expect.objectContaining({ cache: "no-store" }),
    );
    expect(response.status).toBe(200);
    expect(response.headers.get("content-disposition")).toBe('attachment; filename="rec-1.bin"');
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(await response.text()).toBe("recording-bytes");
  });

  it("passes Hub refusals through and rejects path-like ids", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response(JSON.stringify({ error: "recording is still in progress" }), {
          status: 409,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    const conflict = await download("rec-2");
    expect(conflict.status).toBe(409);
    expect(await conflict.json()).toEqual({ error: "recording is still in progress" });

    const malformed = await download("../etc");
    expect(malformed.status).toBe(400);
  });
});
