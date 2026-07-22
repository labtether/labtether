import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { NextRequest } from "next/server";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  backendBaseURLs: vi.fn(() => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { GET, POST } from "../route";

function makeNextRequest(url: string, headers: Record<string, string> = {}): NextRequest {
  return {
    method: "GET",
    headers: new Headers(headers),
    nextUrl: new URL(url),
  } as unknown as NextRequest;
}

describe("settings enrollment route", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("forwards the browser-facing origin on enrollment reads", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ tokens: [] }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));

    const response = await GET(makeNextRequest(
      "http://127.0.0.1:23000/api/settings/enrollment",
      {
        host: "127.0.0.1:23000",
        cookie: "labtether_session=test",
        "x-forwarded-host": "attacker.example",
        "x-forwarded-proto": "https",
      },
    ));

    expect({ status: response.status, body: await response.clone().json() }).toEqual({
      status: 200,
      body: { tokens: [] },
    });
    expect(response.headers.get("Cache-Control")).toBe("no-store");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    const headers = init.headers as Headers;
    expect(headers.get("Cookie")).toBe("labtether_session=test");
    expect(headers.get("Host")).toBeNull();
    expect(headers.get("X-Forwarded-Host")).toBe("127.0.0.1:23000");
    expect(headers.get("X-Forwarded-Proto")).toBe("http");
  });

  it("rejects cross-origin POST requests before proxying", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/settings/enrollment", {
        method: "POST",
        headers: {
          cookie: "labtether_session=test",
          "content-type": "application/json",
          origin: "https://evil.example.com",
        },
        body: JSON.stringify({ label: "test", ttl_hours: 24, max_uses: 1 }),
      }),
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
