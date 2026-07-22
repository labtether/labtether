import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.test", agent: "https://agent.example.test" })),
  upstreamErrorPayload: vi.fn((status: number, payload: unknown, fallback: string) => {
    if (status >= 500) return { error: fallback };
    const record = payload && typeof payload === "object" ? payload as Record<string, unknown> : {};
    return { error: typeof record.error === "string" ? record.error : fallback };
  }),
}));

vi.mock("../../../../../lib/proxyAuth", () => ({
  isMutationRequestOriginAllowed: vi.fn(() => true),
}));

import { GET, POST } from "../route";
import { DELETE, PATCH } from "../[id]/route";
import { maxAPIKeyRequestBodyBytes } from "../proxy";

describe("API key console proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("does not expose backend connection details and marks failures no-store", async () => {
    fetchMock.mockRejectedValueOnce(new Error("connect ECONNREFUSED https://secret.internal:8443"));
    const response = await GET(new Request("https://console.example.test/api/v2/keys"));

    expect(response.status).toBe(502);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(await response.json()).toEqual({ error: "API key endpoint unavailable" });
  });

  it("rejects an oversized chunked create body before contacting the backend", async () => {
    const request = new Request("https://console.example.test/api/v2/keys", {
      method: "POST",
      body: JSON.stringify({ name: "x".repeat(maxAPIKeyRequestBodyBytes) }),
    });
    const response = await POST(request);

    expect(response.status).toBe(413);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("forwards an explicit expiry clear and keeps the response uncacheable", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ data: { status: "updated" } }), { status: 200 }));
    const request = new Request("https://console.example.test/api/v2/keys/key-1", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ expires_at: null, allowed_assets: [] }),
    });
    const response = await PATCH(request as never, { params: Promise.resolve({ id: "key-1" }) });

    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({ expires_at: null, allowed_assets: [] });
  });

  it("rejects malformed key identifiers without issuing a backend request", async () => {
    const response = await DELETE(
      new Request("https://console.example.test/api/v2/keys/bad") as never,
      { params: Promise.resolve({ id: "bad/id" }) },
    );
    expect(response.status).toBe(404);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
