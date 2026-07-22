import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({
    api: "https://api.example.com",
    agent: "https://agent.example.com",
  })),
  upstreamErrorPayload: vi.fn((_status: number, payload: unknown, fallback: string) => {
    if (payload && typeof payload === "object" && !Array.isArray(payload)) return payload;
    return { error: fallback };
  }),
}));

import { GET, POST } from "../route";

describe("SSH hub key settings proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("loads public key metadata with caller session auth and no caching", async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({
      public_key: "ssh-ed25519 AAAA labtether-hub",
      key_type: "ed25519",
      fingerprint_sha256: "SHA256:example",
    }), { status: 200, headers: { "content-type": "application/json" } }));

    const response = await GET(new Request("https://console.example.com/api/settings/ssh-hub-key", {
      headers: { cookie: "labtether_session=test" },
    }));

    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toBe("no-store");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/settings/ssh-hub-key",
      expect.objectContaining({ cache: "no-store", headers: { Cookie: "labtether_session=test" } }),
    );
  });

  it("rejects cross-origin rotation before contacting the backend", async () => {
    const response = await POST(new Request("https://console.example.com/api/settings/ssh-hub-key", {
      method: "POST",
      headers: {
        cookie: "labtether_session=test",
        origin: "https://evil.example.com",
        "content-type": "application/json",
      },
      body: JSON.stringify({ confirm: "ROTATE" }),
    }));

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it.each([
    ["missing confirmation", { key_type: "ed25519" }, 400],
    ["inexact confirmation", { confirm: " ROTATE " }, 400],
    ["unknown key type", { key_type: "dsa", confirm: "ROTATE" }, 400],
    ["control character", { reason: "line one\nline two", confirm: "ROTATE" }, 400],
    ["long reason", { reason: "x".repeat(257), confirm: "ROTATE" }, 400],
  ])("rejects %s", async (_name, body, status) => {
    const response = await POST(new Request("https://console.example.com/api/settings/ssh-hub-key", {
      method: "POST",
      headers: {
        cookie: "labtether_session=test",
        origin: "https://console.example.com",
        "content-type": "application/json",
      },
      body: JSON.stringify(body),
    }));

    expect(response.status).toBe(status);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("forwards only the bounded rotation contract and preserves backend failures", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ error: "existing key remains active" }), {
      status: 502,
      headers: { "content-type": "application/json" },
    }));

    const response = await POST(new Request("https://console.example.com/api/settings/ssh-hub-key", {
      method: "POST",
      headers: {
        cookie: "labtether_session=test",
        origin: "https://console.example.com",
        "content-type": "application/json",
      },
      body: JSON.stringify({
        key_type: " ED25519 ",
        reason: " maintenance window ",
        confirm: "ROTATE",
        private_key: "must-not-forward",
      }),
    }));

    expect(response.status).toBe(502);
    await expect(response.json()).resolves.toEqual({ error: "existing key remains active" });
    const requestOptions = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(requestOptions.body))).toEqual({
      key_type: "ed25519",
      reason: "maintenance window",
      confirm: "ROTATE",
    });
    expect(String(requestOptions.body)).not.toContain("must-not-forward");
  });

  it("returns the rotated public metadata without exposing transport errors", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      status: "rotated",
      key_type: "ed25519",
      fingerprint_sha256: "SHA256:new",
      public_key: "ssh-ed25519 BBBB labtether-hub",
    }), { status: 200, headers: { "content-type": "application/json" } }));

    const response = await POST(new Request("https://console.example.com/api/settings/ssh-hub-key", {
      method: "POST",
      headers: {
        cookie: "labtether_session=test",
        origin: "https://console.example.com",
        "content-type": "application/json",
      },
      body: JSON.stringify({ key_type: "ed25519", reason: "", confirm: "ROTATE" }),
    }));

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toMatchObject({ status: "rotated", fingerprint_sha256: "SHA256:new" });
  });
});
