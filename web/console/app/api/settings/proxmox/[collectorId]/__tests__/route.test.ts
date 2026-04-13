import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { POST } from "../route";

describe("proxmox collector detail route", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("rejects cross-origin POST requests before proxying", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/settings/proxmox/collector-1", {
        method: "POST",
        headers: {
          cookie: "labtether_session=test",
          "content-type": "application/json",
          origin: "https://evil.example.com",
        },
        body: JSON.stringify({ base_url: "https://pve.local:8006", token_id: "labtether@pve!agent" }),
      }),
      { params: Promise.resolve({ collectorId: "collector-1" }) },
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
