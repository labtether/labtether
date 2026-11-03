import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

/** The Hub's audit log a page at a time (limit, offset), for the Audit log page and its CSV export. */
export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const query = new URL(request.url).search;
    const response = await fetch(`${base.api}/api/v2/audit/events${query}`, {
      cache: "no-store",
      headers: backendAuthHeadersWithCookie(request),
      signal: AbortSignal.timeout(15_000),
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? {}, { status: response.status });
  } catch {
    return NextResponse.json({ error: "audit endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
