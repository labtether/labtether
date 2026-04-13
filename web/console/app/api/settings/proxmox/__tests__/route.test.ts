import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { POST } from "../route";

describe("proxmox settings route", () => {
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
      new Request("https://console.example.com/api/settings/proxmox", {
        method: "POST",
        headers: {
          cookie: "labtether_session=test",
          "content-type": "application/json",
          origin: "https://evil.example.com",
        },
        body: JSON.stringify({ base_url: "https://pve.local:8006", token_id: "labtether@pve!agent" }),
      }),
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("cleans up newly created asset and credential when collector creation fails", async () => {
    fetchMock
      .mockResolvedValueOnce(new Response(JSON.stringify({ overrides: {} }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ collectors: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: "asset not found" }), { status: 404 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ status: "accepted" }), { status: 202 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ profile: { id: "cred-proxmox-1" } }), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: "collector create failed" }), { status: 500 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ status: "deleted" }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ status: "deleted" }), { status: 200 }));

    const response = await POST(
      new Request("https://console.example.com/api/settings/proxmox", {
        method: "POST",
        headers: {
          cookie: "labtether_session=test",
          "content-type": "application/json",
          origin: "https://console.example.com",
        },
        body: JSON.stringify({
          base_url: "https://pve.local:8006",
          auth_method: "api_token",
          token_id: "labtether@pve!agent",
          token_secret: "secret",
          cluster_name: "Homelab",
        }),
      }),
    );

    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      "http://localhost:8080/settings/runtime",
      "http://localhost:8080/hub-collectors?enabled=false",
      "http://localhost:8080/assets/proxmox-cluster-homelab",
      "http://localhost:8080/assets/heartbeat",
      "http://localhost:8080/credentials/profiles",
      "http://localhost:8080/hub-collectors",
      "http://localhost:8080/credentials/profiles/cred-proxmox-1",
      "http://localhost:8080/assets/proxmox-cluster-homelab",
    ]);
    expect(response.status).toBe(500);
    expect(fetchMock.mock.calls[6]?.[0]).toBe("http://localhost:8080/credentials/profiles/cred-proxmox-1");
    expect(fetchMock.mock.calls[6]?.[1]).toMatchObject({ method: "DELETE" });
    expect(fetchMock.mock.calls[7]?.[0]).toBe("http://localhost:8080/assets/proxmox-cluster-homelab");
    expect(fetchMock.mock.calls[7]?.[1]).toMatchObject({ method: "DELETE" });
  });
});
