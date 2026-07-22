import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { DELETE, GET, POST } from "../route";

const context = (assetId: string, path: string[]) => ({ params: Promise.resolve({ assetId, path }) });

describe("Portainer asset catch-all route", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("proxies real tab reads and preserves query strings", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ data: [] }), {
      status: 200,
      headers: { "content-type": "application/json", "content-encoding": "gzip" },
    }));
    const response = await GET(
      new Request("https://console.example.com/api/portainer/assets/asset-1/containers?all=true"),
      context("asset-1", ["containers"]),
    );

    expect(response.status).toBe(200);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.example.com/portainer/assets/asset-1/containers?all=true");
    expect(response.headers.get("content-encoding")).toBeNull();
  });

  it("forwards same-origin mutations and their bodies", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ data: { status: "ok" } }), { status: 200 }));
    await POST(
      new Request("https://console.example.com/api/portainer/assets/asset-1/networks", {
        method: "POST",
        headers: { origin: "https://console.example.com", "content-type": "application/json" },
        body: JSON.stringify({ name: "backend" }),
      }),
      context("asset-1", ["networks"]),
    );

    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe("POST");
    expect(new TextDecoder().decode(fetchMock.mock.calls[0]?.[1]?.body as ArrayBuffer)).toBe('{"name":"backend"}');
  });

  it("rejects cross-origin mutations and non-tab paths before proxying", async () => {
    const crossOrigin = await DELETE(
      new Request("https://console.example.com/api/portainer/assets/asset-1/images/image-1", {
        method: "DELETE",
        headers: { origin: "https://evil.example.com" },
      }),
      context("asset-1", ["images", "image-1"]),
    );
    const disallowed = await GET(
      new Request("https://console.example.com/api/portainer/assets/asset-1/credentials"),
      context("asset-1", ["credentials"]),
    );

    expect(crossOrigin.status).toBe(403);
    expect(disallowed.status).toBe(404);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects dot segments and decoded separators before constructing the upstream URL", async () => {
    const escapedAsset = await GET(
      new Request("https://console.example.com/api/portainer/assets/ignored/overview"),
      context("..", ["overview", "..", "..", "metrics"]),
    );
    const escapedPath = await GET(
      new Request("https://console.example.com/api/portainer/assets/asset-1/overview"),
      context("asset-1", ["overview", "../settings"]),
    );

    expect(escapedAsset.status).toBe(404);
    expect(escapedPath.status).toBe(404);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects an oversized chunked mutation body before proxying", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/portainer/assets/asset-1/networks", {
        method: "POST",
        headers: { origin: "https://console.example.com", "content-type": "application/json" },
        body: JSON.stringify({ name: "backend", ignored: "x".repeat(1024 * 1024) }),
      }),
      context("asset-1", ["networks"]),
    );

    expect(response.status).toBe(413);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
