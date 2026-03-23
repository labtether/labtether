import { describe, it, expect } from "vitest";
import {
  hasLabtetherSessionCookie,
  isMutationRequestOriginAllowed,
} from "../../../lib/proxyAuth";

/**
 * Helper to build a minimal request object matching the RequestLike shape
 * used by isMutationRequestOriginAllowed.
 */
function makeRequest(
  method: string,
  url: string,
  headerEntries: Record<string, string> = {}
) {
  const headers = new Headers(headerEntries);
  return { method, url, headers };
}

// ---------------------------------------------------------------------------
// hasLabtetherSessionCookie
// ---------------------------------------------------------------------------
describe("hasLabtetherSessionCookie", () => {
  it("returns true for exact cookie name", () => {
    expect(hasLabtetherSessionCookie("labtether_session=abc")).toBe(true);
  });

  it("returns false for partial/prefixed cookie name", () => {
    expect(hasLabtetherSessionCookie("old_labtether_session=foo")).toBe(false);
  });

  it("returns true when session cookie is among multiple cookies", () => {
    expect(hasLabtetherSessionCookie("foo=bar; labtether_session=abc")).toBe(
      true
    );
  });

  it("returns true when session cookie is first among multiple cookies", () => {
    expect(hasLabtetherSessionCookie("labtether_session=abc; foo=bar")).toBe(
      true
    );
  });

  it("returns false for null cookie header", () => {
    expect(hasLabtetherSessionCookie(null)).toBe(false);
  });

  it("returns false for empty cookie header", () => {
    expect(hasLabtetherSessionCookie("")).toBe(false);
  });

  it("returns false when no matching cookie exists", () => {
    expect(hasLabtetherSessionCookie("other=value; another=thing")).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// isMutationRequestOriginAllowed — CSRF logic
// ---------------------------------------------------------------------------
describe("isMutationRequestOriginAllowed", () => {
  const SAME_ORIGIN = "https://hub.example.com";

  describe("service token CSRF bypass fix", () => {
    it("denies browser request with service token and bad origin", () => {
      const req = makeRequest("POST", `${SAME_ORIGIN}/api/action`, {
        "x-labtether-token": "svc-token-123",
        origin: "https://evil.example.com",
        host: "hub.example.com",
      });
      expect(isMutationRequestOriginAllowed(req)).toBe(false);
    });

    it("allows non-browser request (no Origin) with service token", () => {
      const req = makeRequest("POST", `${SAME_ORIGIN}/api/action`, {
        "x-labtether-token": "svc-token-123",
        host: "hub.example.com",
      });
      expect(isMutationRequestOriginAllowed(req)).toBe(true);
    });

    it("allows browser request with service token and matching origin", () => {
      const req = makeRequest("POST", `${SAME_ORIGIN}/api/action`, {
        "x-labtether-token": "svc-token-123",
        origin: SAME_ORIGIN,
        host: "hub.example.com",
      });
      expect(isMutationRequestOriginAllowed(req)).toBe(true);
    });
  });

  describe("session cookie origin validation", () => {
    it("allows browser request with session cookie and valid origin", () => {
      const req = makeRequest("POST", `${SAME_ORIGIN}/api/action`, {
        cookie: "labtether_session=abc123",
        origin: SAME_ORIGIN,
        host: "hub.example.com",
      });
      expect(isMutationRequestOriginAllowed(req)).toBe(true);
    });

    it("denies browser request with session cookie and invalid origin", () => {
      const req = makeRequest("POST", `${SAME_ORIGIN}/api/action`, {
        cookie: "labtether_session=abc123",
        origin: "https://evil.example.com",
        host: "hub.example.com",
      });
      expect(isMutationRequestOriginAllowed(req)).toBe(false);
    });
  });

  describe("non-mutating methods", () => {
    it("allows GET requests regardless of origin", () => {
      const req = makeRequest("GET", `${SAME_ORIGIN}/api/data`, {
        origin: "https://evil.example.com",
        host: "hub.example.com",
      });
      expect(isMutationRequestOriginAllowed(req)).toBe(true);
    });
  });

  describe("no origin header, no service token", () => {
    it("allows when sec-fetch-site is same-origin", () => {
      const req = makeRequest("POST", `${SAME_ORIGIN}/api/action`, {
        "sec-fetch-site": "same-origin",
        host: "hub.example.com",
      });
      expect(isMutationRequestOriginAllowed(req)).toBe(true);
    });

    it("denies when sec-fetch-site is cross-site", () => {
      const req = makeRequest("POST", `${SAME_ORIGIN}/api/action`, {
        "sec-fetch-site": "cross-site",
        host: "hub.example.com",
      });
      expect(isMutationRequestOriginAllowed(req)).toBe(false);
    });
  });
});
