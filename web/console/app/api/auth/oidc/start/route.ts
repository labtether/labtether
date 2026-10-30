import { NextResponse } from "next/server";
import { resolvedBackendBaseURLs, shouldUseSecureWebSocket } from "../../../../../lib/backend";
import { safeLocalRedirectPath } from "../../../../../lib/safeRedirect";
import { markResponseNoStore } from "../../../../../lib/noStoreResponse";
import { oidcStateFromAuthURL, setOIDCStateCookie } from "../../../../../lib/oidcStateCookie";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const requestURL = new URL(request.url);
  const host = request.headers.get("host") ?? requestURL.host;
  const origin = `${requestURL.protocol}//${host}`;
  const base = await resolvedBackendBaseURLs();
  const redirectURI = `${origin}/api/auth/oidc/callback`;
  const nextPath = safeLocalRedirectPath(requestURL.searchParams.get("next"));

  try {
    const response = await fetch(`${base.api}/auth/oidc/start`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ redirect_uri: redirectURI, next: nextPath }),
      cache: "no-store",
    });

    const payload = await safeJSON(response) as { auth_url?: string } | null;
    if (!response.ok || !payload?.auth_url) {
      return markResponseNoStore(NextResponse.redirect(new URL("/login?error=sso_unavailable", requestURL)));
    }

    const state = oidcStateFromAuthURL(payload.auth_url);
    if (!state) {
      return markResponseNoStore(NextResponse.redirect(new URL("/login?error=sso_unavailable", requestURL)));
    }
    const redirect = markResponseNoStore(NextResponse.redirect(payload.auth_url));
    setOIDCStateCookie(redirect, state, shouldUseSecureWebSocket(request));
    return redirect;
  } catch {
    return markResponseNoStore(NextResponse.redirect(new URL("/login?error=sso_unavailable", requestURL)));
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
