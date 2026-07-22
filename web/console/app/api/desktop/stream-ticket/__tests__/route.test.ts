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
  shouldUseSecureWebSocket: vi.fn(() => true),
  upstreamErrorPayload: vi.fn(
    (status: number, payload: { error?: string }, fallback: string) => ({
      error: status >= 500 ? fallback : payload.error || fallback,
    }),
  ),
}));

import { POST } from "../route";

describe("desktop stream-ticket proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () =>
      new Response(
        JSON.stringify({
          ticket: "ticket-1",
          stream_path: "/desktop/sessions/session-1/stream?ticket=ticket-1",
          vnc_password: "  exact password\n",
        }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("returns an exact credential-bearing ticket only through a no-store response", async () => {
    const response = await POST(
      new NextRequest("https://console.example.com/api/desktop/stream-ticket", {
        method: "POST",
        headers: {
          origin: "https://console.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({ sessionId: "session-1" }),
      }),
    );

    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(await response.json()).toMatchObject({
      sessionId: "session-1",
      vncPassword: "  exact password\n",
      secure: true,
    });
  });

  it("rejects oversized or malformed session IDs before proxying", async () => {
    for (const body of [
      JSON.stringify({ sessionId: "x".repeat(5000) }),
      JSON.stringify({ sessionId: "bad/id" }),
    ]) {
      const response = await POST(
        new NextRequest("https://console.example.com/api/desktop/stream-ticket", {
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
