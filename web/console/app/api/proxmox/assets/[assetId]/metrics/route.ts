import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";

export const dynamic = "force-dynamic";

type Params = {
  params: Promise<{
    assetId: string;
  }>;
};

export async function GET(request: Request, { params }: Params) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId } = await params;
  const assetID = encodeURIComponent(assetId ?? "");
  const incomingURL = new URL(request.url);
  const timeframe = incomingURL.searchParams.get("timeframe")?.trim() ?? "hour";
  const collectorID = incomingURL.searchParams.get("collector_id")?.trim() ?? "";

  const query = new URLSearchParams();
  query.set("timeframe", timeframe);
  if (collectorID !== "") {
    query.set("collector_id", collectorID);
  }

  try {
    const response = await fetch(
      `${base.api}/proxmox/assets/${assetID}/metrics?${query.toString()}`,
      { cache: "no-store", headers: authHeaders, signal: AbortSignal.timeout(15_000) },
    );
    const payload = await response.json().catch(() => null);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to fetch metrics" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to fetch metrics" },
      { status: 502 },
    );
  }
}
