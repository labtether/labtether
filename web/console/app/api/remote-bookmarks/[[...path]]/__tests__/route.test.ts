import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({
    api: "https://api.example.com",
    agent: "https://agent.example.com",
  })),
}));

import { DELETE, GET, POST, PUT } from "../route";

describe("remote bookmark proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("preserves a successful empty DELETE response", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

    const response = await DELETE(
      new NextRequest("https://console.example.com/api/remote-bookmarks/bookmark-1", {
        method: "DELETE",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://console.example.com",
        },
      }),
      { params: Promise.resolve({ path: ["bookmark-1"] }) },
    );

    expect(response.status).toBe(204);
    expect(await response.text()).toBe("");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/api/v1/remote-bookmarks/bookmark-1",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it.each([
    ["POST", POST],
    ["PUT", PUT],
    ["DELETE", DELETE],
  ])("rejects cross-origin %s before proxying", async (method, handler) => {
    const response = await handler(
      new NextRequest("https://console.example.com/api/remote-bookmarks/bookmark-1", {
        method,
        headers: {
          cookie: "labtether_session=test",
          origin: "https://evil.example.com",
          "content-type": "application/json",
        },
        body: method === "DELETE" ? undefined : JSON.stringify({ label: "test" }),
      }),
      { params: Promise.resolve({ path: ["bookmark-1"] }) },
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("marks credential reveal responses and the upstream fetch as no-store", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ username: "qa", password: "secret" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));

    const response = await GET(
      new NextRequest("https://console.example.com/api/remote-bookmarks/bookmark-1/credentials", {
        headers: { cookie: "labtether_session=test" },
      }),
      { params: Promise.resolve({ path: ["bookmark-1", "credentials"] }) },
    );

    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/api/v1/remote-bookmarks/bookmark-1/credentials",
      expect.objectContaining({ cache: "no-store" }),
    );
  });
});
