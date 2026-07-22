import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { POST } from "../route";

describe("terminal persistent sessions collection route", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("proxies session creation to the registered hub method", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ persistent_session: { id: "pts-1" } }), {
      status: 201,
    }));
    const response = await POST(new Request("https://console.example.com/api/terminal/persistent-sessions", {
      method: "POST",
      headers: {
        cookie: "labtether_session=test",
        origin: "https://console.example.com",
        "content-type": "application/json",
      },
      body: JSON.stringify({ target: " node-1 ", title: " Shell ", actor_id: "spoofed" }),
    }));

    expect(response.status).toBe(201);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.example.com/terminal/persistent-sessions");
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe("POST");
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      target: "node-1",
      title: "Shell",
    });
  });

  it("rejects cross-origin creation before proxying", async () => {
    const response = await POST(new Request("https://console.example.com/api/terminal/persistent-sessions", {
      method: "POST",
      headers: {
        cookie: "labtether_session=test",
        origin: "https://evil.example.com",
        "content-type": "application/json",
      },
      body: JSON.stringify({ target: "node-1" }),
    }));

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects an oversized chunked body before parsing or proxying", async () => {
    const response = await POST(new Request("https://console.example.com/api/terminal/persistent-sessions", {
      method: "POST",
      headers: {
        cookie: "labtether_session=test",
        origin: "https://console.example.com",
        "content-type": "application/json",
      },
      body: JSON.stringify({ target: "node-1", ignored: "x".repeat(65 * 1024) }),
    }));

    expect(response.status).toBe(413);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
