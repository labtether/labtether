// @vitest-environment node

import { unstable_doesMiddlewareMatch as unstable_doesProxyMatch } from "next/experimental/testing/server";
import { NextRequest } from "next/server";
import { describe, expect, it, vi } from "vitest";

// The matcher is static configuration. Avoid loading next-intl's runtime
// middleware (which expects Next's production ESM resolver) in this Node test.
vi.mock("next-intl/middleware", () => ({
  default: () => () => undefined,
}));

import { config, proxy } from "../../proxy";

describe("proxy matcher", () => {
  it("bypasses proxy.ts for streaming file transfers", () => {
    expect(unstable_doesProxyMatch({ config, url: "https://hub.example/api/files/asset-1/upload" })).toBe(false);
  });

  it("bypasses locale and session middleware for the MCP endpoint only", () => {
    expect(unstable_doesProxyMatch({ config, url: "https://hub.example/mcp" })).toBe(false);
    expect(unstable_doesProxyMatch({ config, url: "https://hub.example/mcp/admin" })).toBe(true);
    expect(unstable_doesProxyMatch({ config, url: "https://hub.example/mcpx" })).toBe(true);
  });

  it.each([
    "https://hub.example/api/auth/login",
    "https://hub.example/api/assets",
    "https://hub.example/en/dashboard",
  ])("continues to protect %s", (url) => {
    expect(unstable_doesProxyMatch({ config, url })).toBe(true);
  });
});

describe("proxy API guard", () => {
  // /api-docs is a console page; only /api and /api/... are hub API calls that need a session.
  it("lets the API Docs page through to page routing", () => {
    expect(proxy(new NextRequest("https://hub.example/api-docs"))).toBeUndefined();
  });

  it("still refuses API calls without a session", () => {
    expect(proxy(new NextRequest("https://hub.example/api/assets"))?.status).toBe(401);
  });
});
