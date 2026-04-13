import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type Params = {
  params: Promise<{
    node: string;
    upid: string;
  }>;
};

export async function POST(request: Request, { params }: Params) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { node, upid } = await params;
  const incomingURL = new URL(request.url);
  const collectorID = incomingURL.searchParams.get("collector_id")?.trim() ?? "";
  const query = new URLSearchParams();
  if (collectorID !== "") {
    query.set("collector_id", collectorID);
  }
  const querySuffix = query.size > 0 ? `?${query.toString()}` : "";

  try {
    const response = await fetch(
      `${base.api}/pbs/tasks/${encodeURIComponent(node)}/${encodeURIComponent(upid)}/stop${querySuffix}`,
      { method: "POST", cache: "no-store", headers: authHeaders, signal: AbortSignal.timeout(10_000) },
    );
    const payload = await response.json().catch(() => null);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to stop pbs task" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to stop pbs task" },
      { status: 502 },
    );
  }
}
