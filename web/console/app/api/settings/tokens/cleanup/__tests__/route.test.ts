import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  backendBaseURLs: vi.fn(() => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { DELETE } from "../route";

describe("settings token cleanup route", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("rejects cross-origin DELETE requests before proxying", async () => {
    const response = await DELETE(
      new Request("https://console.example.com/api/settings/tokens/cleanup", {
        method: "DELETE",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://evil.example.com",
        },
      }),
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("proxies cleanup requests to the backend", async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ enrollment_deleted: 2, agent_deleted: 1 }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );

    const response = await DELETE(
      new Request("https://console.example.com/api/settings/tokens/cleanup", {
        method: "DELETE",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://console.example.com",
        },
      }),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ enrollment_deleted: 2, agent_deleted: 1 });
    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining("/settings/tokens/cleanup"), expect.objectContaining({
      method: "DELETE",
      cache: "no-store",
    }));
  });
});
