import { describe, expect, it } from "vitest";
import { demoEntryRedirect } from "../demoEntry";

const visit = (over: Partial<Parameters<typeof demoEntryRedirect>[0]> = {}) =>
  demoEntryRedirect({ demoMode: true, method: "GET", pathname: "/en/nodes", search: "", cookieHeader: null, ...over });

// The public demo must open on the console, not on a sign-in form, and must
// never bounce a visitor between the sign-in page and the session route.
describe("demo entry", () => {
  it("sends a visitor without a session through the demo session route and back to the page", () => {
    expect(visit({ search: "?group=type" })).toBe("/api/demo/session?redirect=%2Fen%2Fnodes%3Fgroup%3Dtype");
    expect(visit({ pathname: "/" })).toBe("/api/demo/session?redirect=%2F");
  });

  it("leaves a visitor who has a session alone", () => {
    expect(visit({ cookieHeader: "NEXT_LOCALE=en; labtether_session=abc" })).toBeNull();
  });

  it("never redirects the sign-in page, which is the fallback when no session can be made", () => {
    expect(visit({ pathname: "/login" })).toBeNull();
    expect(visit({ pathname: "/de/login", search: "?demo=manual" })).toBeNull();
  });

  it("does nothing on a normal hub", () => {
    expect(visit({ demoMode: false })).toBeNull();
  });

  it("does not turn a form post into a session request", () => {
    expect(visit({ method: "POST" })).toBeNull();
  });
});
