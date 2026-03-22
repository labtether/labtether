import { NextResponse } from "next/server";
import { resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

function safeNextPath(value: unknown): string {
  if (typeof value !== "string" || !value.startsWith("/")) return "/";
  if (value.startsWith("//")) return "/";
  const lower = value.toLowerCase();
  if (lower.includes("javascript:") || lower.includes("data:")) return "/";
  return value;
}

export async function GET(request: Request) {
  const requestURL = new URL(request.url);
  const host = request.headers.get("host") ?? requestURL.host;
  const origin = `${requestURL.protocol}//${host}`;
  const code = requestURL.searchParams.get("code")?.trim() ?? "";
  const state = requestURL.searchParams.get("state")?.trim() ?? "";

  if (!code || !state) {
    return NextResponse.redirect(new URL("/login?error=Missing%20OIDC%20callback%20parameters", origin));
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

    const payload = await safeJSON(response) as { next?: string; error?: string } | null;
    if (!response.ok) {
      return NextResponse.redirect(new URL(`/login?error=${encodeURIComponent(payload?.error ?? "SSO login failed")}`, origin));
    }

    const nextPath = safeNextPath(payload?.next);
    const redirect = NextResponse.redirect(new URL(nextPath, origin));
    const setCookie = response.headers.get("set-cookie");
    if (setCookie) {
      redirect.headers.set("set-cookie", setCookie);
    }
    return redirect;
  } catch {
    return NextResponse.redirect(new URL("/login?error=SSO%20login%20failed", origin));
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
