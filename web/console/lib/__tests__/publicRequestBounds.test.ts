// @vitest-environment node
import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { proxyPublicHubRequest } from "../publicHubProxy";
import { POST as login } from "../../app/api/auth/login/route";
import { POST as login2FA } from "../../app/api/auth/login/2fa/route";
import { POST as bootstrap } from "../../app/api/auth/bootstrap/route";

vi.mock("../backend", () => ({
  resolvedBackendBaseURLs: async () => ({ api: "https://hub.example.test" }),
  shouldUseSecureWebSocket: () => true,
  upstreamErrorPayload: (_status: number, payload: unknown) => payload,
}));
vi.mock("../proxyAuth", () => ({ isMutationRequestOriginAllowed: () => true }));
vi.mock("../rateLimit", () => ({ checkRateLimit: () => ({ success: true }) }));

beforeEach(() => vi.stubGlobal("fetch", vi.fn(async () => new Response("{}", { status: 200 }))));
afterEach(() => vi.unstubAllGlobals());

function oversizedRequest(path: string, length?: string) {
  const cancel = vi.fn();
  let reads = 0;
  const stream = new ReadableStream<Uint8Array>({
    pull(controller) {
      reads++;
      controller.enqueue(new Uint8Array(64 * 1024));
    },
    cancel,
  }, { highWaterMark: 0 });
  const headers = new Headers({ "content-type": "application/json" });
  if (length !== undefined) headers.set("content-length", length);
  const request = new NextRequest(`https://console.example.test/${path}`, {
    method: "POST", headers, body: stream, duplex: "half",
  } as NonNullable<ConstructorParameters<typeof NextRequest>[1]>);
  return { request, cancel, reads: () => reads };
}

describe("public request stream bounds", () => {
  const routes = [
    ["login", login], ["login/2fa", login2FA], ["bootstrap", bootstrap],
    ["public proxy", (request: Request) => proxyPublicHubRequest(request as NextRequest, "/api/v1/discover")],
  ] as const;

  for (const [name, route] of routes) {
    it.each([undefined, "1"])(`${name} stops an oversized stream with content-length %s`, async (length) => {
      const source = oversizedRequest(name, length);
      const response = await route(source.request);
      expect(response.status).toBe(413);
      expect(source.cancel).toHaveBeenCalledOnce();
      expect(source.reads()).toBe(17);
      expect(fetch).not.toHaveBeenCalled();
    });
  }

  it.each([login, login2FA, bootstrap])("rejects malformed auth JSON as a client error", async (route) => {
    const request = new Request("https://console.example.test/auth", { method: "POST", body: "{" });
    expect((await route(request)).status).toBe(400);
    expect(fetch).not.toHaveBeenCalled();
  });

  it.each([
    [login, { username: "owner", password: "fake-test-password" }, "/auth/login"],
    [login2FA, { challenge_token: "fake-test-challenge", code: "123456" }, "/auth/login/2fa"],
    [bootstrap, { username: "owner", password: "fake-test-password", setup_token: "fake-test-setup" }, "/auth/bootstrap"],
  ] as const)("forwards valid auth JSON", async (route, body, path) => {
    const request = new Request("https://console.example.test/auth", { method: "POST", body: JSON.stringify(body) });
    expect((await route(request)).status).toBe(200);
    expect(fetch).toHaveBeenCalledWith(`https://hub.example.test${path}`, expect.objectContaining({ method: "POST" }));
  });

  it("forwards a public body exactly at the limit", async () => {
    const body = "x".repeat(1 << 20);
    const request = new NextRequest("https://console.example.test/discover", { method: "POST", body });
    expect((await proxyPublicHubRequest(request, "/api/v1/discover")).status).toBe(200);
    const init = vi.mocked(fetch).mock.calls[0][1];
    expect(new TextDecoder().decode(init?.body as ArrayBuffer)).toBe(body);
  });
});
