import { NextRequest, NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: NextRequest) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const host = request.nextUrl.searchParams.get("host");
    const includeHidden = request.nextUrl.searchParams.get("include_hidden");
    const connector = request.nextUrl.searchParams.get("connector");
    const minConfidence = request.nextUrl.searchParams.get("min_confidence");

    const params = new URLSearchParams();
    if (host) params.set("host", host);
    if (includeHidden) params.set("include_hidden", includeHidden);
    if (connector) params.set("connector", connector);
    if (minConfidence) params.set("min_confidence", minConfidence);

    const qs = params.toString() ? `?${params.toString()}` : "";

    const response = await fetch(`${base.api}/api/v1/services/web/compat${qs}`, {
      cache: "no-store",
      headers: authHeaders,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to load compatible APIs" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "request failed" },
      { status: 502 }
    );
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
