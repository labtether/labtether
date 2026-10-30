import { timingSafeEqual } from "node:crypto";
import type { NextResponse } from "next/server";

// SSO sign-in is bound to the browser that started it. The start route keeps
// the OIDC state in this cookie and the callback accepts only a matching
// state, so a callback link made in someone else's browser cannot sign this
// browser in to their account.
export const OIDC_STATE_COOKIE = "labtether_oidc_state";
// The callback is a redirect from the identity provider and carries no Origin.
// A browser keeps this Secure cookie only on a secure page, so its absence on
// the callback shows that the sign-in ran over plain HTTP.
export const OIDC_SECURE_PROBE_COOKIE = "labtether_oidc_tls";
const OIDC_STATE_PATH = "/api/auth/oidc";
const OIDC_STATE_MAX_AGE_SECONDS = 10 * 60;

export function oidcStateFromAuthURL(authURL: string): string {
  try {
    return new URL(authURL).searchParams.get("state")?.trim() ?? "";
  } catch {
    return "";
  }
}

export function setOIDCStateCookie(response: NextResponse, state: string, secure: boolean): void {
  response.cookies.set(OIDC_STATE_COOKIE, state, {
    httpOnly: true,
    sameSite: "lax",
    secure,
    path: OIDC_STATE_PATH,
    maxAge: OIDC_STATE_MAX_AGE_SECONDS,
  });
  response.cookies.set(OIDC_SECURE_PROBE_COOKIE, "1", {
    httpOnly: true,
    sameSite: "lax",
    secure: true,
    path: OIDC_STATE_PATH,
    maxAge: OIDC_STATE_MAX_AGE_SECONDS,
  });
}

export function clearOIDCStateCookie(response: NextResponse, secure: boolean): void {
  response.cookies.set(OIDC_STATE_COOKIE, "", {
    httpOnly: true,
    sameSite: "lax",
    secure,
    path: OIDC_STATE_PATH,
    maxAge: 0,
  });
  response.cookies.set(OIDC_SECURE_PROBE_COOKIE, "", {
    httpOnly: true,
    sameSite: "lax",
    secure: true,
    path: OIDC_STATE_PATH,
    maxAge: 0,
  });
}

export function readOIDCStateCookie(request: Request): string {
  return readCookie(request, OIDC_STATE_COOKIE);
}

// The protocol the browser used for this sign-in, as far as cookies can tell.
export function oidcCallbackPageProtocol(request: Request): string {
  return readCookie(request, OIDC_SECURE_PROBE_COOKIE) ? "https:" : "http:";
}

function readCookie(request: Request, name: string): string {
  const header = request.headers.get("cookie") ?? "";
  for (const part of header.split(";")) {
    const separator = part.indexOf("=");
    if (separator < 0) continue;
    if (part.slice(0, separator).trim() !== name) continue;
    const raw = part.slice(separator + 1).trim();
    try {
      return decodeURIComponent(raw);
    } catch {
      return "";
    }
  }
  return "";
}

export function oidcStateMatches(cookieState: string, queryState: string): boolean {
  if (!cookieState || !queryState) return false;
  const expected = Buffer.from(cookieState);
  const actual = Buffer.from(queryState);
  return expected.length === actual.length && timingSafeEqual(expected, actual);
}
