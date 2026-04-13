import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  backendBaseURLs: vi.fn(() => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { DELETE } from "../route";

describe("settings agent token route", () => {
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
      new Request("https://console.example.com/api/settings/agent-tokens/atok-1", {
        method: "DELETE",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://evil.example.com",
        },
      }),
      { params: Promise.resolve({ id: "atok-1" }) },
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
