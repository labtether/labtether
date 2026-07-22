import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({
    api: "https://api.example.com",
    agent: "https://agent.example.com",
  })),
  upstreamErrorPayload: vi.fn((status: number, payload: { error?: string }, fallback: string) => ({
    error: status >= 500 ? fallback : payload.error || fallback,
  })),
}));

import { POST } from "../route";

describe("desktop session proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ id: "session-direct", target: "192.0.2.70:3389", mode: "desktop" }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("forwards a structured direct target without composing an ambiguous host:port target", async () => {
    const response = await POST(
      new NextRequest("https://console.example.com/api/desktop/session", {
        method: "POST",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://console.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({
          protocol: "rdp",
          direct_target: {
            host: "2001:db8::70",
            port: 3389,
            username: "qa-user",
            password: "synthetic-secret",
          },
        }),
      }),
    );

    expect(response.status).toBe(201);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(JSON.parse(String(init.body))).toMatchObject({
      protocol: "rdp",
      direct_target: {
        host: "2001:db8::70",
        port: 3389,
        username: "qa-user",
        password: "synthetic-secret",
      },
    });
    expect(JSON.parse(String(init.body)).target).toBeUndefined();
  });

  it("rejects malformed direct target types before proxying", async () => {
    const response = await POST(
      new NextRequest("https://console.example.com/api/desktop/session", {
        method: "POST",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://console.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({
          protocol: "rdp",
          direct_target: { host: "192.0.2.70", port: "3389" },
        }),
      }),
    );

    expect(response.status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("preserves exact password bytes while rejecting invalid protocol and port values", async () => {
    const password = "  exact secret\n";
    const accepted = await POST(
      new NextRequest("https://console.example.com/api/desktop/session", {
        method: "POST",
        headers: {
          origin: "https://console.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({
          protocol: "rdp",
          direct_target: {
            host: "192.0.2.70",
            port: 3389,
            password,
          },
        }),
      }),
    );
    expect(accepted.status).toBe(201);
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(JSON.parse(String(init.body)).direct_target.password).toBe(password);

    fetchMock.mockClear();
    for (const body of [
      { protocol: "telnet", target: "asset-1" },
      {
        protocol: "rdp",
        direct_target: { host: "192.0.2.70", port: 70_000 },
      },
    ]) {
      const response = await POST(
        new NextRequest("https://console.example.com/api/desktop/session", {
          method: "POST",
          headers: {
            origin: "https://console.example.com",
            "content-type": "application/json",
          },
          body: JSON.stringify(body),
        }),
      );
      expect(response.status).toBe(400);
    }
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("stream-bounds request bodies and redacts upstream failures", async () => {
    const oversized = await POST(
      new NextRequest("https://console.example.com/api/desktop/session", {
        method: "POST",
        headers: {
          origin: "https://console.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({ target: "x".repeat(40_000) }),
      }),
    );
    expect(oversized.status).toBe(413);
    expect(fetchMock).not.toHaveBeenCalled();

    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "internal credential path" }), {
        status: 500,
        headers: { "Content-Type": "application/json" },
      }),
    );
    const failed = await POST(
      new NextRequest("https://console.example.com/api/desktop/session", {
        method: "POST",
        headers: {
          origin: "https://console.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({ target: "asset-1" }),
      }),
    );
    expect(failed.status).toBe(500);
    expect(await failed.json()).toEqual({
      error: "failed to create remote view session",
    });
  });

  it("rejects a cross-origin direct session before proxying credentials", async () => {
    const response = await POST(
      new NextRequest("https://console.example.com/api/desktop/session", {
        method: "POST",
        headers: {
          cookie: "labtether_session=test",
          origin: "https://evil.example.com",
          "content-type": "application/json",
        },
        body: JSON.stringify({
          protocol: "rdp",
          direct_target: { host: "192.0.2.70", port: 3389, password: "synthetic-secret" },
        }),
      }),
    );

    expect(response.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
