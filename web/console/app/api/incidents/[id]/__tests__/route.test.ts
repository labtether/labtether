import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
  upstreamErrorPayload: vi.fn((_status: number, payload: unknown, fallback: string) => payload ?? { error: fallback }),
}));

import { PATCH } from "../route";

describe("incident detail route", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("rejects cross-origin PATCH requests before proxying", async () => {
    const response = await PATCH(
      new Request("https://console.example.com/api/incidents/inc-1", {
        method: "PATCH",
        headers: {
          cookie: "labtether_session=test",
          "content-type": "application/json",
          origin: "https://evil.example.com",
        },
        body: JSON.stringify({ status: "resolved" }),
      }),
      { params: Promise.resolve({ id: "inc-1" }) },
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
