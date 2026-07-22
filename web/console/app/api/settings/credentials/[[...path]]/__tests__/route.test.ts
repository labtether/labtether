import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
  upstreamErrorPayload: vi.fn((_status: number, _payload: unknown, fallback: string) => ({ error: fallback })),
}));

import { DELETE, GET, maxCredentialRequestBodyBytes, POST } from "../route";

const context = (path?: string[]) => ({ params: Promise.resolve({ path }) });

describe("credential profile settings proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("lists redacted profiles through a no-store bounded upstream request", async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ profiles: [{ id: "cred_1", name: "SSH" }] }), { status: 200 }));
    const response = await GET(new Request("https://console.example.com/api/settings/credentials?limit=9999"), context());

    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/api/v2/credentials/profiles?limit=500",
      expect.objectContaining({ cache: "no-store", method: "GET" }),
    );
  });

  it("preserves exact secret and passphrase bytes while proxying create", async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ profile: { id: "cred_1" } }), { status: 201 }));
    const payload = JSON.stringify({
      name: "SSH key",
      kind: "ssh_private_key",
      secret: " \nprivate key\t ",
      passphrase: " passphrase ",
    });
    const response = await POST(new Request("https://console.example.com/api/settings/credentials", {
      method: "POST",
      headers: {
        cookie: "labtether_session=test",
        origin: "https://console.example.com",
        "content-type": "application/json",
      },
      body: payload,
    }), context());

    expect(response.status).toBe(201);
    const forwarded = fetchMock.mock.calls[0]?.[1]?.body as Uint8Array;
    expect(new TextDecoder().decode(forwarded)).toBe(payload);
  });

  it("rejects cross-origin mutation, oversized bodies, and unsafe IDs locally", async () => {
    const crossOrigin = await DELETE(new Request("https://console.example.com/api/settings/credentials/cred_1", {
      method: "DELETE",
      headers: { cookie: "labtether_session=test", origin: "https://evil.example.com" },
    }), context(["cred_1"]));
    expect(crossOrigin.status).toBe(403);

    const oversized = await POST(new Request("https://console.example.com/api/settings/credentials", {
      method: "POST",
      headers: { origin: "https://console.example.com" },
      body: "x".repeat(maxCredentialRequestBodyBytes + 1),
    }), context());
    expect(oversized.status).toBe(413);

    const unsafe = await DELETE(new Request("https://console.example.com/api/settings/credentials/unsafe", {
      method: "DELETE",
      headers: { origin: "https://console.example.com" },
    }), context([".."]));
    expect(unsafe.status).toBe(404);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("preserves redacted dependency conflicts for honest delete feedback", async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({
      error: "credential profile is in use",
      reference_count: 2,
      references: [{ resource: "asset_protocol_configs", count: 2 }],
    }), { status: 409 }));
    const response = await DELETE(new Request("https://console.example.com/api/settings/credentials/cred_1", {
      method: "DELETE",
      headers: { cookie: "labtether_session=test", origin: "https://console.example.com" },
    }), context(["cred_1"]));

    expect(response.status).toBe(409);
    expect(await response.json()).toMatchObject({
      reference_count: 2,
      references: [{ resource: "asset_protocol_configs", count: 2 }],
    });
  });
});
