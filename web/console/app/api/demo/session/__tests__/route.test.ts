import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  resolvedBackendBaseURLs: vi.fn(async () => ({
    api: "https://api.example.com",
    agent: "https://agent.example.com",
  })),
}));

import { GET, HEAD } from "../route";

describe("demo session route", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("probes demo mode with HEAD without creating a session", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));

    const response = await HEAD();

    expect(response.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/api/demo/session",
      expect.objectContaining({ method: "HEAD", redirect: "manual" }),
    );
  });

  it("normalizes disabled demo mode to a successful no-content probe", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 404 }));

    const response = await HEAD();

    expect(response.status).toBe(204);
  });

  it("propagates a disabled demo endpoint instead of fabricating a redirect", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 404 }));

    const response = await GET(
      new Request("https://console.example.com/api/demo/session?redirect=%2Fen"),
    );

    expect(response.status).toBe(404);
    expect(response.headers.get("location")).toBeNull();
  });
});
