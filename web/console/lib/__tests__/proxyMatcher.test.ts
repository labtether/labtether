// @vitest-environment node

import { unstable_doesMiddlewareMatch as unstable_doesProxyMatch } from "next/experimental/testing/server";
import { describe, expect, it, vi } from "vitest";

// The matcher is static configuration. Avoid loading next-intl's runtime
// middleware (which expects Next's production ESM resolver) in this Node test.
vi.mock("next-intl/middleware", () => ({
  default: () => () => undefined,
}));

import { config } from "../../proxy";

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
