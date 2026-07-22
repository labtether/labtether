import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
  upstreamErrorPayload: vi.fn((_status: number, payload: unknown, fallback: string) => payload ?? { error: fallback }),
}));

import { DELETE } from "../route";

describe("user deletion proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("preserves a successful empty 204 response without constructing a JSON body", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

    const response = await DELETE(
      new Request("https://console.example.com/api/auth/users/usr-1", {
        method: "DELETE",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://console.example.com",
        },
      }),
      { params: Promise.resolve({ id: "usr-1" }) },
    );

    expect(response.status).toBe(204);
    expect(await response.text()).toBe("");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/auth/users/usr-1",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it("rejects cross-origin deletion before proxying", async () => {
    const response = await DELETE(
      new Request("https://console.example.com/api/auth/users/usr-1", {
        method: "DELETE",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://evil.example.com",
        },
      }),
      { params: Promise.resolve({ id: "usr-1" }) },
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
