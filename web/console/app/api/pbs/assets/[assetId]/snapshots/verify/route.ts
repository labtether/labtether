import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type Params = { params: Promise<{ assetId: string }> };

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}

export async function POST(request: Request, { params }: Params) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId } = await params;
  const assetID = encodeURIComponent(assetId ?? "");

  let body: unknown;
  try { body = await request.json(); } catch { body = undefined; }

  const incomingURL = new URL(request.url);
  const bodyRecord = body && typeof body === "object" ? body as Record<string, unknown> : {};
  const store = incomingURL.searchParams.get("store")?.trim()
    || (typeof bodyRecord.store === "string" ? bodyRecord.store.trim() : "");
  const upstreamURL = new URL(`${base.api}/pbs/assets/${assetID}/snapshots/verify`);
  if (store) upstreamURL.searchParams.set("store", store);

  try {
    const response = await fetch(upstreamURL, {
      method: "POST",
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body ?? {}),
      signal: AbortSignal.timeout(30_000),
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to verify snapshot" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "failed to verify snapshot" }, { status: 502 });
  }
}
