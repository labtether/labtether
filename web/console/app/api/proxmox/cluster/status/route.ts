import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const incomingURL = new URL(request.url);
  const collectorID = incomingURL.searchParams.get("collector_id")?.trim() ?? "";
  const query = new URLSearchParams();
  if (collectorID !== "") {
    query.set("collector_id", collectorID);
  }
  const querySuffix = query.size > 0 ? `?${query.toString()}` : "";

  try {
    const response = await fetch(`${base.api}/proxmox/cluster/status${querySuffix}`, {
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(10_000),
    });
    const payload = await response.json().catch(() => null);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to fetch cluster status" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to fetch cluster status" },
      { status: 502 },
    );
  }
}
