import { NextRequest, NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: NextRequest) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const host = request.nextUrl.searchParams.get("host");
    const includeHidden = request.nextUrl.searchParams.get("include_hidden");
    const detail = request.nextUrl.searchParams.get("detail");
    const serviceID = request.nextUrl.searchParams.get("service_id");
    const params = new URLSearchParams();
    if (host) params.set("host", host);
    if (includeHidden) params.set("include_hidden", includeHidden);
    if (detail) params.set("detail", detail);
    if (serviceID) params.set("service_id", serviceID);
    const qs = params.toString() ? `?${params.toString()}` : "";

    const response = await fetch(`${base.api}/api/v1/services/web${qs}`, {
      cache: "no-store",
      headers: authHeaders,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to load web services" }, { status: response.status });
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
