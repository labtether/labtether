import { NextResponse } from "next/server";
import { resolvedBackendBaseURLs, shouldUseSecureWebSocket } from "../../../../../lib/backend";
import { safeLocalRedirectPath } from "../../../../../lib/safeRedirect";
import { markResponseNoStore } from "../../../../../lib/noStoreResponse";
import { clearOIDCStateCookie, oidcStateMatches, readOIDCStateCookie } from "../../../../../lib/oidcStateCookie";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const requestURL = new URL(request.url);
  const host = request.headers.get("host") ?? requestURL.host;
  const origin = `${requestURL.protocol}//${host}`;
  const code = requestURL.searchParams.get("code")?.trim() ?? "";
  const state = requestURL.searchParams.get("state")?.trim() ?? "";
  const secure = shouldUseSecureWebSocket(request);
  const finish = (response: NextResponse) => {
    clearOIDCStateCookie(response, secure);
    return markResponseNoStore(response);
  };

  if (!code || !state) {
    return finish(NextResponse.redirect(new URL("/login?error=sso_expired", origin)));
  }
  // Only the browser that started this sign-in may finish it.
  if (!oidcStateMatches(readOIDCStateCookie(request), state)) {
    return finish(NextResponse.redirect(new URL("/login?error=sso_expired", origin)));
  }

  const base = await resolvedBackendBaseURLs();
  const redirectURI = `${origin}/api/auth/oidc/callback`;

  try {
    const response = await fetch(`${base.api}/auth/oidc/callback`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code, state, redirect_uri: redirectURI }),
      cache: "no-store",
    });

    const payload = await safeJSON(response) as { next?: string } | null;
    if (!response.ok) {
      return finish(NextResponse.redirect(new URL(`/login?error=${callbackErrorCode(response.status)}`, origin)));
    }

    const nextPath = safeLocalRedirectPath(payload?.next);
    const redirect = NextResponse.redirect(new URL(nextPath, origin));
    // Clear the state cookie before adding the session cookie: setting a
    // cookie through NextResponse rewrites every Set-Cookie header it did not
    // add itself.
    clearOIDCStateCookie(redirect, secure);
    for (const cookie of response.headers.getSetCookie()) {
      redirect.headers.append("set-cookie", cookie);
    }
    return markResponseNoStore(redirect);
  } catch {
    return finish(NextResponse.redirect(new URL("/login?error=sso_failed", origin)));
  }
}

// The login page shows a fixed message for each code. It never shows text
// taken from the URL.
function callbackErrorCode(status: number): string {
  if (status === 400) return "sso_expired";
  if (status === 409) return "sso_setup_required";
  return "sso_failed";
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
