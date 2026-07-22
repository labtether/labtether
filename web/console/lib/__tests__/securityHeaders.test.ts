import { describe, expect, it } from "vitest";

import { consoleContentSecurityPolicy, consoleSecurityHeaders } from "../securityHeaders";

describe("console security headers", () => {
  it("locks the production console to same-origin resources and non-embeddable content", () => {
    const csp = consoleContentSecurityPolicy(false);

    for (const directive of [
      "default-src 'self'",
      "script-src-attr 'none'",
      "object-src 'none'",
      "base-uri 'self'",
      "form-action 'self'",
      "frame-src 'none'",
      "frame-ancestors 'none'",
      "connect-src 'self'",
    ]) {
      expect(csp).toContain(directive);
    }
    expect(csp).not.toContain("'unsafe-eval'");
    expect(csp).not.toContain(" ws:");
    expect(csp).not.toContain(" wss:");
  });

  it("adds only local development websocket and eval allowances in development", () => {
    const csp = consoleContentSecurityPolicy(true);
    expect(csp).toContain("'unsafe-eval'");
    expect(csp).toContain("ws://localhost:*");
    expect(csp).toContain("ws://127.0.0.1:*");
  });

  it("emits the CSP alongside defense-in-depth response headers", () => {
    const headers = new Map(consoleSecurityHeaders(false).map(({ key, value }) => [key, value]));
    expect(headers.get("Content-Security-Policy")).toBeTruthy();
    expect(headers.get("X-Content-Type-Options")).toBe("nosniff");
    expect(headers.get("X-Frame-Options")).toBe("DENY");
  });
});
