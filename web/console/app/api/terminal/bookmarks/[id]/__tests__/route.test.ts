import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { DELETE, PUT } from "../route";

describe("terminal bookmark route", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("rejects cross-origin PUT requests before proxying", async () => {
    const response = await PUT(
      new Request("https://console.example.com/api/terminal/bookmarks/bkm-1", {
        method: "PUT",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://evil.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({ title: "Updated" }),
      }),
      { params: Promise.resolve({ id: "bkm-1" }) },
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects cross-origin DELETE requests before proxying", async () => {
    const response = await DELETE(
      new Request("https://console.example.com/api/terminal/bookmarks/bkm-1", {
        method: "DELETE",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://evil.example.com",
        },
      }),
      { params: Promise.resolve({ id: "bkm-1" }) },
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
