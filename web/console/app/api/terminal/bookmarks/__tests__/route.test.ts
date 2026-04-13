import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { POST } from "../route";

describe("terminal bookmarks route", () => {
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
      new Request("https://console.example.com/api/terminal/bookmarks", {
        method: "POST",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://evil.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({ title: "Lab Host", host: "lab-host-01", port: 22 }),
      }),
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
