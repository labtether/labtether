import { NextResponse } from "next/server";
import { resolvedBackendBaseURLs, backendAuthHeadersWithCookie } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/settings/oidc`, {
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(10_000),
    });
    const payload = await response.json();
    return NextResponse.json(payload, { status: response.status });
  } catch {
    return NextResponse.json({ error: "failed to load oidc settings" }, { status: 502 });
  }
}

export async function PUT(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const body = await request.json();
    const response = await fetch(`${base.api}/settings/oidc`, {
      method: "PUT",
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(10_000),
    });
    const payload = await response.json();
    return NextResponse.json(payload, { status: response.status });
  } catch {
    return NextResponse.json({ error: "failed to save oidc settings" }, { status: 502 });
  }
}
