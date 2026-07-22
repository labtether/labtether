import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { PATCH } from "../route";

describe("notification channel proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;
  const context = { params: Promise.resolve({ channelId: "channel/one" }) };

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("rejects non-object JSON before proxying", async () => {
    const response = await PATCH(new Request("https://console.example.com/api/notifications/channels/channel", {
      method: "PATCH",
      headers: { origin: "https://console.example.com", "content-type": "application/json" },
      body: "[]",
    }), context);

    expect(response.status).toBe(400);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("encodes the channel id and preserves the backend status", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ channel: { id: "channel/one" } }), { status: 200 }));
    const response = await PATCH(new Request("https://console.example.com/api/notifications/channels/channel", {
      method: "PATCH",
      headers: { origin: "https://console.example.com", "content-type": "application/json" },
      body: JSON.stringify({ enabled: false }),
    }), context);

    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.example.com/notifications/channels/channel%2Fone");
  });
});
