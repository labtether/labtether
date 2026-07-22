import { afterEach, describe, expect, it, vi } from "vitest";

import { requestNotificationChannelTest } from "../useNotificationChannels";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("notification channel test request", () => {
  it("returns a bounded failure instead of throwing on a network error", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("connection refused")));

    await expect(requestNotificationChannelTest("channel/one")).resolves.toEqual({
      success: false,
      error: "connection refused",
    });
    expect(fetch).toHaveBeenCalledWith(
      "/api/notifications/channels/channel%2Fone/test",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("does not claim success for malformed successful JSON", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("not-json", { status: 200 })));

    await expect(requestNotificationChannelTest("channel-1")).resolves.toEqual({
      success: false,
      error: "test delivery was not confirmed",
    });
  });

  it("sanitizes credential text returned by a failed provider test", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      success: false,
      error: "authorization=Bearer super-secret",
    }), { status: 200 })));

    const result = await requestNotificationChannelTest("channel-1");
    expect(result.success).toBe(false);
    expect(result.error).not.toContain("super-secret");
    expect(result.error).toContain("[redacted]");
  });
});
