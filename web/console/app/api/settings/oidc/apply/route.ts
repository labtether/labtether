import { NextResponse } from "next/server";
import { resolvedBackendBaseURLs, backendAuthHeadersWithCookie } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/settings/oidc/apply`, {
      method: "POST",
      headers: authHeaders,
      signal: AbortSignal.timeout(20_000),
    });
    const payload = await response.json();
    return NextResponse.json(payload, { status: response.status });
  } catch {
    return NextResponse.json({ error: "failed to apply oidc settings" }, { status: 502 });
  }
}
