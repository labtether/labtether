import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
  upstreamErrorPayload: vi.fn((_status: number, payload: unknown, fallback: string) => payload ?? { error: fallback }),
}));

import { POST } from "../route";

describe("current-user password route", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("proxies the exact supported password fields to the registered hub route", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ status: "updated" }), { status: 200 }));
    const response = await POST(new Request("https://console.example.com/api/auth/me/password", {
      method: "POST",
      headers: { origin: "https://console.example.com", "content-type": "application/json" },
      body: JSON.stringify({ current_password: "current", new_password: "replacement", ignored: "value" }),
    }));

    expect(response.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.example.com/auth/me/password");
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      current_password: "current",
      new_password: "replacement",
    });
  });

  it("rejects cross-origin password changes before proxying", async () => {
    const response = await POST(new Request("https://console.example.com/api/auth/me/password", {
      method: "POST",
      headers: { origin: "https://evil.example.com", "content-type": "application/json" },
      body: JSON.stringify({ current_password: "current", new_password: "replacement" }),
    }));

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects an oversized chunked body before parsing or proxying", async () => {
    const response = await POST(new Request("https://console.example.com/api/auth/me/password", {
      method: "POST",
      headers: { origin: "https://console.example.com", "content-type": "application/json" },
      body: JSON.stringify({
        current_password: "current",
        new_password: "replacement",
        ignored: "x".repeat(9 * 1024),
      }),
    }));

    expect(response.status).toBe(413);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
