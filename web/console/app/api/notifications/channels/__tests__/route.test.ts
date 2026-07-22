import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { GET, POST } from "../route";

describe("notification channels proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("rejects malformed JSON without proxying and marks the response no-store", async () => {
    const response = await POST(new Request("https://console.example.com/api/notifications/channels", {
      method: "POST",
      headers: { origin: "https://console.example.com", "content-type": "application/json" },
      body: "{broken",
    }));

    expect(response.status).toBe(400);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects an oversized declared body before proxying", async () => {
    const response = await POST(new Request("https://console.example.com/api/notifications/channels", {
      method: "POST",
      headers: {
        origin: "https://console.example.com",
        "content-type": "application/json",
        "content-length": String(97 * 1024),
      },
      body: "{}",
    }));

    expect(response.status).toBe(413);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("proxies valid JSON and marks sensitive channel responses no-store", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ channel: { id: "channel-1" } }), { status: 201 }));
    const response = await POST(new Request("https://console.example.com/api/notifications/channels", {
      method: "POST",
      headers: { origin: "https://console.example.com", "content-type": "application/json" },
      body: JSON.stringify({ name: "Ops", type: "webhook", config: { url: "https://hooks.example.com" } }),
    }));

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/notifications/channels",
      expect.objectContaining({ method: "POST", cache: "no-store" }),
    );
  });

  it("treats a malformed successful backend response as unavailable", async () => {
    fetchMock.mockResolvedValueOnce(new Response("not-json", { status: 200 }));
    const response = await GET(new Request("https://console.example.com/api/notifications/channels"));

    expect(response.status).toBe(502);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(await response.json()).toEqual({ error: "notification backend unavailable" });
  });
});
