import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { POST } from "../route";

describe("notification channel test proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;
  const context = { params: Promise.resolve({ channelId: "channel-1" }) };

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("does not report success for a malformed backend response", async () => {
    fetchMock.mockResolvedValueOnce(new Response("", { status: 200 }));
    const response = await POST(new Request("https://console.example.com/api/notifications/channels/channel-1/test", {
      method: "POST",
      headers: { origin: "https://console.example.com" },
    }), context);

    expect(response.status).toBe(502);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(await response.json()).toEqual({ success: false, error: "notification backend unavailable" });
  });

  it("rejects a cross-origin test before proxying", async () => {
    const response = await POST(new Request("https://console.example.com/api/notifications/channels/channel-1/test", {
      method: "POST",
      headers: { origin: "https://evil.example.com" },
    }), context);

    expect(response.status).toBe(403);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
