import { hasLabtetherSessionCookie } from "./proxyAuth";

const LOGIN_PATH = /^(?:\/(?:en|de|fr|es|zh))?\/login\/?$/;

/**
 * Where a page request goes first on the public demo.
 *
 * The demo signs every visitor in as its read-only viewer. A page request
 * that carries no session is sent through the demo session route, which sets
 * the session and comes back to the page, so a visitor never meets a sign-in
 * form. The sign-in page itself is left alone: it is where the session route
 * sends a visitor when it cannot make a session, and redirecting it again
 * would loop.
 *
 * Returns a path relative to the console, or null to carry on as usual.
 */
export function demoEntryRedirect(input: {
  demoMode: boolean;
  method: string;
  pathname: string;
  search: string;
  cookieHeader: string | null;
}): string | null {
  if (!input.demoMode || input.method !== "GET") return null;
  if (hasLabtetherSessionCookie(input.cookieHeader)) return null;
  if (LOGIN_PATH.test(input.pathname)) return null;
  const page = `${input.pathname}${input.search}`;
  return `/api/demo/session?redirect=${encodeURIComponent(page)}`;
}
