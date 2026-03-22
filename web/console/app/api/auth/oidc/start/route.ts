import { NextResponse } from "next/server";
import { resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

function safeNextPath(value: string | null): string {
  if (!value || !value.startsWith("/")) return "/";
  if (value.startsWith("//")) return "/";
  const lower = value.toLowerCase();
  if (lower.includes("javascript:") || lower.includes("data:")) return "/";
  return value;
}

export async function GET(request: Request) {
  const requestURL = new URL(request.url);
  const host = request.headers.get("host") ?? requestURL.host;
  const origin = `${requestURL.protocol}//${host}`;
  const base = await resolvedBackendBaseURLs();
  const redirectURI = `${origin}/api/auth/oidc/callback`;
  const nextPath = safeNextPath(requestURL.searchParams.get("next"));

  try {
    const response = await fetch(`${base.api}/auth/oidc/start`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ redirect_uri: redirectURI, next: nextPath }),
      cache: "no-store",
    });

    const payload = await safeJSON(response) as { auth_url?: string; error?: string } | null;
    if (!response.ok || !payload?.auth_url) {
      return NextResponse.redirect(new URL(`/login?error=${encodeURIComponent(payload?.error ?? "SSO unavailable")}`, requestURL));
    }

    return NextResponse.redirect(payload.auth_url);
  } catch {
    return NextResponse.redirect(new URL("/login?error=SSO%20unavailable", requestURL));
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
