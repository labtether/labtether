import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({
    api: "https://api.example.com",
    agent: "https://agent.example.com",
  })),
  shouldUseSecureWebSocket: vi.fn(() => true),
  upstreamErrorPayload: vi.fn((status: number, payload: { error?: string }, fallback: string) => ({
    error: status >= 500 ? fallback : payload.error || fallback,
  })),
}));

import { POST } from "../route";

describe("desktop SPICE ticket proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () =>
      new Response(
        JSON.stringify({
          session_id: "session-spice",
          ticket: "one-time-ticket",
          stream_path: "/desktop/sessions/session-spice/stream?ticket=one-time-ticket&protocol=spice",
          password: "",
          type: "spice",
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

  it("preserves an explicit empty password for a passwordless direct SPICE endpoint", async () => {
    const response = await POST(
      new NextRequest("https://console.example.com/api/desktop/spice-ticket", {
        method: "POST",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://console.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({ sessionId: "session-spice" }),
      }),
    );

    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(await response.json()).toMatchObject({
      sessionId: "session-spice",
      password: "",
      secure: true,
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("rejects a cross-origin request before fetching the credential-bearing ticket", async () => {
    const response = await POST(
      new NextRequest("https://console.example.com/api/desktop/spice-ticket", {
        method: "POST",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://evil.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({ sessionId: "session-spice" }),
      }),
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
