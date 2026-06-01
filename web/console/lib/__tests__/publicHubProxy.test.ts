import { afterEach, describe, expect, it, vi } from "vitest";
import type { NextRequest } from "next/server";
import { proxyPublicHubRequest, validatePublicContentLength } from "../publicHubProxy";

function makePublicRequest(url: string, headers: Record<string, string> = {}): NextRequest {
  return {
    method: "GET",
    headers: new Headers(headers),
    nextUrl: new URL(url),
  } as unknown as NextRequest;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("validatePublicContentLength", () => {
  it("allows missing and in-limit content lengths", () => {
    expect(validatePublicContentLength(null)).toEqual({ ok: true });
    expect(validatePublicContentLength("")).toEqual({ ok: true });
    expect(validatePublicContentLength("1048576")).toEqual({ ok: true });
  });

  it("rejects malformed decimal lengths", () => {
    expect(validatePublicContentLength("1e3")).toEqual({
      ok: false,
      status: 400,
      message: "invalid content-length",
    });
    expect(validatePublicContentLength("30abc")).toEqual({
      ok: false,
      status: 400,
      message: "invalid content-length",
    });
    expect(validatePublicContentLength("-1")).toEqual({
      ok: false,
      status: 400,
      message: "invalid content-length",
    });
  });

  it("rejects oversized declared bodies before reading", () => {
    expect(validatePublicContentLength("1048577")).toEqual({
      ok: false,
      status: 413,
      message: "request body too large",
    });
    expect(validatePublicContentLength("999999999999999999999999")).toEqual({
      ok: false,
      status: 413,
      message: "request body too large",
    });
  });
});

describe("proxyPublicHubRequest", () => {
  it("does not trust client-supplied forwarded host or proto headers", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response("ok", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const request = makePublicRequest("https://console.example.test/api/v1/discover?probe=1", {
      host: "console.example.test",
      "x-forwarded-host": "attacker.example",
      "x-forwarded-proto": "http",
    });

    await proxyPublicHubRequest(request, "/api/v1/discover");

    const proxyCall = fetchMock.mock.calls.find(([input]) => String(input).includes("/api/v1/discover"));
    if (!proxyCall || !proxyCall[1]) {
      throw new Error("expected proxy fetch call");
    }
    const [, init] = proxyCall;
    const headers = init.headers as Headers;
    expect(headers.get("Host")).toBe("console.example.test");
    expect(headers.get("X-Forwarded-Host")).toBe("console.example.test");
    expect(headers.get("X-Forwarded-Proto")).toBe("https");
  });
});
