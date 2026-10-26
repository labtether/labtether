import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=fixture" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://hub.example.com" })),
}));

import { POST } from "../route";

const context = (action: string) => ({
  params: Promise.resolve({ assetId: "nas-1", snapshotId: "tank/data@before upgrade", action }),
});

function request(body?: string, origin = "https://console.example.com") {
  return new Request("https://console.example.com/api/truenas/assets/nas-1/snapshots/fixture/clone", {
    method: "POST", headers: { origin, "content-type": "application/json" }, body,
  });
}

describe("TrueNAS snapshot action proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () => new Response('{"status":"ok"}', { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("forwards the clone destination with its exact encoded snapshot and auth", async () => {
    const body = JSON.stringify({ dataset_dst: "dest/parent/restored" });
    const response = await POST(request(body), context("clone"));
    expect(response.status).toBe(200);
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toBe("https://hub.example.com/truenas/assets/nas-1/snapshots/tank%2Fdata%40before%20upgrade/clone");
    expect(init).toEqual(expect.objectContaining({
      method: "POST", headers: { Cookie: "labtether_session=fixture", "Content-Type": "application/json" },
    }));
    expect(new TextDecoder().decode(init?.body as ArrayBuffer)).toBe(body);
  });

  it("keeps rollback bodyless and preserves Hub rejection", async () => {
    fetchMock.mockResolvedValueOnce(new Response('{"error":"maintenance window"}', { status: 423 }));
    const response = await POST(request(), context("rollback"));
    expect(response.status).toBe(423);
    expect(await response.json()).toEqual({ error: "maintenance window" });
    expect(fetchMock.mock.calls[0]?.[1]?.body).toBeUndefined();
  });

  it("rejects untrusted origins and unknown actions before sending a mutation", async () => {
    expect((await POST(request("{}", "https://other.example.com"), context("clone"))).status).toBe(403);
    expect((await POST(request("{}"), context("destroy"))).status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("bounds clone input before forwarding it", async () => {
    const response = await POST(request("x".repeat(1024 * 1024 + 1)), context("clone"));
    expect(response.status).toBe(413);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
