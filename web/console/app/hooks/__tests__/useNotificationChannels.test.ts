import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { fetchAllNotificationChannels, requestNotificationChannelTest, useNotificationChannels } from "../useNotificationChannels";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const channels = Array.from({ length: 105 }, (_, index) => ({
  id: `channel-${index}`, name: `Channel ${index}`, type: "webhook", config: {},
  enabled: false, created_at: "2026-10-09T00:00:00Z", updated_at: "2026-10-09T00:00:00Z",
}));

function channelPage(offset: number, duplicate = false): Response {
  const page = channels.slice(offset, offset + 100);
  return new Response(JSON.stringify({
    channels: duplicate && offset === 100 ? [channels[99], ...page.slice(1)] : page,
    capabilities: { smtp_insecure_transport_allowed: false },
  }), { status: 200 });
}

function ChannelCount() {
  const { channels: loaded, loading, error } = useNotificationChannels();
  return createElement("div", null, loading ? "Loading" : error || `${loaded.length} channels`);
}

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

describe("Settings notification channel list", () => {
  it("loads every channel when the Hub has 105", async () => {
    const fetchMock = vi.fn(async (url: string) => {
      const offset = Number(new URL(url, "http://localhost").searchParams.get("offset"));
      return channelPage(offset);
    });
    vi.stubGlobal("fetch", fetchMock);
    const container = document.createElement("div");
    document.body.append(container);
    const root = createRoot(container);
    try {
      await act(async () => root.render(createElement(ChannelCount)));

      expect(container.textContent).toBe("105 channels");
      expect(fetchMock).toHaveBeenCalledTimes(2);
      expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
        "/api/notifications/channels?limit=100&offset=0",
        "/api/notifications/channels?limit=100&offset=100",
      ]);
    } finally {
      await act(async () => root.unmount());
      container.remove();
    }
  });

  it("shows an error instead of a partial list when a later page fails", async () => {
    vi.stubGlobal("fetch", vi.fn(async (url: string) => {
      const offset = Number(new URL(url, "http://localhost").searchParams.get("offset"));
      return offset === 100 ? new Response("{}", { status: 503 }) : channelPage(offset);
    }));
    const container = document.createElement("div");
    document.body.append(container);
    const root = createRoot(container);
    try {
      await act(async () => root.render(createElement(ChannelCount)));

      expect(container.textContent).toContain("failed to load notification channels (503)");
      expect(container.textContent).not.toContain("100 channels");
    } finally {
      await act(async () => root.unmount());
      container.remove();
    }
  });

  it("rejects a repeated first page if the page offset is lost", async () => {
    vi.stubGlobal("fetch", vi.fn(async (url: string) => {
      const offset = Number(new URL(url, "http://localhost").searchParams.get("offset"));
      return channelPage(offset, true);
    }));

    await expect(fetchAllNotificationChannels()).rejects.toThrow("incomplete notification channels response (offset 100)");
  });
});
