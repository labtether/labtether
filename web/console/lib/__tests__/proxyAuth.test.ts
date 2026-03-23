import { describe, it, expect } from "vitest";
import {
  hasLabtetherSessionCookie,
  hasServiceModeAuthHeader,
  isMutationRequestOriginAllowed,
  isMutatingMethod,
  isTrustedServiceProxyPath,
  isProxyRequestAuthorized,
} from "../proxyAuth";

/**
 * Helper to build a Request-like object for testing isMutationRequestOriginAllowed.
 * The url must be absolute for the URL constructor used inside the implementation.
 */
function makeRequest(
  method: string,
  opts: {
    origin?: string;
    cookie?: string;
    serviceToken?: string;
    authorization?: string;
    host?: string;
    secFetchSite?: string;
    forwardedProto?: string;
    forwardedHost?: string;
  } = {}
): Request {
  const headers = new Headers();
  if (opts.origin !== undefined) headers.set("origin", opts.origin);
  if (opts.cookie !== undefined) headers.set("cookie", opts.cookie);
  if (opts.serviceToken !== undefined)
    headers.set("x-labtether-token", opts.serviceToken);
  if (opts.authorization !== undefined)
    headers.set("authorization", opts.authorization);
  if (opts.host !== undefined) headers.set("host", opts.host);
  if (opts.secFetchSite !== undefined)
    headers.set("sec-fetch-site", opts.secFetchSite);
  if (opts.forwardedProto !== undefined)
    headers.set("x-forwarded-proto", opts.forwardedProto);
  if (opts.forwardedHost !== undefined)
    headers.set("x-forwarded-host", opts.forwardedHost);

  return {
    method,
    url: "http://hub.local:3000/api/assets",
    headers,
  } as unknown as Request;
}

// ---------------------------------------------------------------------------
// hasLabtetherSessionCookie
// ---------------------------------------------------------------------------

describe("hasLabtetherSessionCookie", () => {
  it("returns false for null", () => {
    expect(hasLabtetherSessionCookie(null)).toBe(false);
  });

  it("returns false when cookie is missing", () => {
    expect(hasLabtetherSessionCookie("other=abc")).toBe(false);
  });

  it("returns true when session cookie is present alone", () => {
    expect(hasLabtetherSessionCookie("labtether_session=abc123")).toBe(true);
  });

  it("returns true when session cookie is among others", () => {
    expect(
      hasLabtetherSessionCookie("foo=bar; labtether_session=abc123; baz=qux")
    ).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// hasServiceModeAuthHeader
// ---------------------------------------------------------------------------

describe("hasServiceModeAuthHeader", () => {
  it("returns true with x-labtether-token", () => {
    const h = new Headers();
    h.set("x-labtether-token", "secret");
    expect(hasServiceModeAuthHeader(h)).toBe(true);
  });

  it("returns true with authorization bearer", () => {
    const h = new Headers();
    h.set("authorization", "Bearer tok");
    expect(hasServiceModeAuthHeader(h)).toBe(true);
  });

  it("returns false with empty headers", () => {
    expect(hasServiceModeAuthHeader(new Headers())).toBe(false);
  });

  it("returns false with blank token", () => {
    const h = new Headers();
    h.set("x-labtether-token", "   ");
    expect(hasServiceModeAuthHeader(h)).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// isMutationRequestOriginAllowed — CSRF protection tests
// ---------------------------------------------------------------------------

describe("isMutationRequestOriginAllowed", () => {
  // Test 1: Browser request with service token MUST validate origin
  it("denies browser POST with service token and mismatched origin", () => {
    const req = makeRequest("POST", {
      serviceToken: "my-service-token",
      origin: "https://evil.com",
      host: "hub.local:3000",
    });
    expect(isMutationRequestOriginAllowed(req)).toBe(false);
  });

  // Test 2: Non-browser request with service token (no Origin) -> allow
  it("allows non-browser POST with service token and no Origin header", () => {
    const req = makeRequest("POST", {
      serviceToken: "my-service-token",
      // No origin header — CLI / agent behavior
    });
    expect(isMutationRequestOriginAllowed(req)).toBe(true);
  });

  // Test 3: Browser + session cookie + valid (same) origin -> allow
  it("allows browser POST with session cookie and same origin", () => {
    const req = makeRequest("POST", {
      cookie: "labtether_session=abc123",
      origin: "http://hub.local:3000",
      host: "hub.local:3000",
    });
    expect(isMutationRequestOriginAllowed(req)).toBe(true);
  });

  // Test 4: Browser + session cookie + invalid origin -> deny
  it("denies browser POST with session cookie and cross-origin", () => {
    const req = makeRequest("POST", {
      cookie: "labtether_session=abc123",
      origin: "https://evil.com",
      host: "hub.local:3000",
    });
    expect(isMutationRequestOriginAllowed(req)).toBe(false);
  });

  // Test 5: Browser + no auth at all -> deny (unless same-origin signals)
  it("denies browser POST with no auth and cross-origin", () => {
    const req = makeRequest("POST", {
      origin: "https://evil.com",
      host: "hub.local:3000",
    });
    expect(isMutationRequestOriginAllowed(req)).toBe(false);
  });

  // Supplementary: GET requests are always allowed regardless of origin
  it("allows GET requests regardless of origin", () => {
    const req = makeRequest("GET", {
      origin: "https://evil.com",
      host: "hub.local:3000",
    });
    expect(isMutationRequestOriginAllowed(req)).toBe(true);
  });

  // Supplementary: No origin + same-origin sec-fetch-site signals -> allow
  it("allows POST with no origin and sec-fetch-site=same-origin", () => {
    const req = makeRequest("POST", {
      cookie: "labtether_session=abc123",
      secFetchSite: "same-origin",
      host: "hub.local:3000",
    });
    expect(isMutationRequestOriginAllowed(req)).toBe(true);
  });

  // Service token + Origin matching request host -> allow
  it("allows service token POST when origin matches host", () => {
    const req = makeRequest("POST", {
      serviceToken: "my-service-token",
      origin: "http://hub.local:3000",
      host: "hub.local:3000",
    });
    expect(isMutationRequestOriginAllowed(req)).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// isTrustedServiceProxyPath
// ---------------------------------------------------------------------------

describe("isTrustedServiceProxyPath", () => {
  it("returns true for /api/status", () => {
    expect(isTrustedServiceProxyPath("/api/status")).toBe(true);
  });

  it("returns true for /api/status/live", () => {
    expect(isTrustedServiceProxyPath("/api/status/live")).toBe(true);
  });

  it("returns false for /api/assets", () => {
    expect(isTrustedServiceProxyPath("/api/assets")).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// isMutatingMethod
// ---------------------------------------------------------------------------

describe("isMutatingMethod", () => {
  it("returns true for POST, PUT, PATCH, DELETE", () => {
    for (const m of ["POST", "PUT", "PATCH", "DELETE"]) {
      expect(isMutatingMethod(m)).toBe(true);
    }
  });

  it("returns false for GET, HEAD, OPTIONS", () => {
    for (const m of ["GET", "HEAD", "OPTIONS"]) {
      expect(isMutatingMethod(m)).toBe(false);
    }
  });
});

// ---------------------------------------------------------------------------
// isProxyRequestAuthorized
// ---------------------------------------------------------------------------

describe("isProxyRequestAuthorized", () => {
  it("allows session cookie on any path", () => {
    const h = new Headers();
    h.set("cookie", "labtether_session=abc123");
    expect(isProxyRequestAuthorized("/api/assets", h)).toBe(true);
  });

  it("denies service token on non-trusted path", () => {
    const h = new Headers();
    h.set("x-labtether-token", "secret");
    expect(isProxyRequestAuthorized("/api/assets", h)).toBe(false);
  });

  it("allows service token on trusted path", () => {
    const h = new Headers();
    h.set("x-labtether-token", "secret");
    expect(isProxyRequestAuthorized("/api/status", h)).toBe(true);
  });

  it("denies unauthenticated request", () => {
    expect(isProxyRequestAuthorized("/api/assets", new Headers())).toBe(false);
  });
});
