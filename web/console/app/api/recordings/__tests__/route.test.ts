import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../lib/backend", () => ({
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

describe("recording start proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () =>
      new Response(
        JSON.stringify({ recording_id: "recording-1", status: "recording" }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("forwards only the authoritative session identifier and disables caching", async () => {
    const response = await POST(
      new NextRequest("https://console.example.com/api/recordings", {
        method: "POST",
        headers: {
          origin: "https://console.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({
          session_id: "session-1",
          asset_id: "spoofed-asset",
          protocol: "spoofed-protocol",
        }),
      }),
    );

    expect(response.status).toBe(201);
    expect(response.headers.get("cache-control")).toContain("no-store");
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(JSON.parse(String(init.body))).toEqual({ session_id: "session-1" });
  });

  it("rejects oversized and malformed requests before proxying", async () => {
    for (const body of [
      JSON.stringify({ session_id: "x".repeat(5000) }),
      JSON.stringify({ session_id: "bad/id" }),
    ]) {
      const response = await POST(
        new NextRequest("https://console.example.com/api/recordings", {
          method: "POST",
          headers: {
            origin: "https://console.example.com",
            "content-type": "application/json",
          },
          body,
        }),
      );
      expect([400, 413]).toContain(response.status);
    }
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
