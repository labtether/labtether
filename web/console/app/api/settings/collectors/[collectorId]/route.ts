import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

type HubCollector = {
  id: string;
  asset_id?: string;
  collector_type: string;
  last_status?: string;
  last_error?: string;
  last_collected_at?: string;
};

type Asset = {
  id: string;
  metadata?: Record<string, string>;
};

export async function GET(
  request: Request,
  { params }: { params: Promise<{ collectorId: string }> },
) {
  const { collectorId } = await params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  if (!collectorId.trim()) {
    return NextResponse.json({ error: "collector id is required" }, { status: 400 });
  }

  try {
    const collectorRes = await fetch(
      `${base.api}/hub-collectors/${encodeURIComponent(collectorId)}`,
      { cache: "no-store", headers: authHeaders, signal: AbortSignal.timeout(10_000) },
    );
    const collectorPayload = (await safeJSON(collectorRes)) as { collector?: HubCollector; error?: string } | null;
    if (!collectorRes.ok) {
      return NextResponse.json(
        collectorPayload ?? { error: "failed to load collector" },
        { status: collectorRes.status },
      );
    }

    const collector = collectorPayload?.collector;
    if (!collector) {
      return NextResponse.json({ error: "collector not found" }, { status: 404 });
    }

    let discovered: number | undefined;
    const assetID = collector.asset_id?.trim() ?? "";
    if (assetID) {
      const assetRes = await fetch(
        `${base.api}/assets/${encodeURIComponent(assetID)}`,
        { cache: "no-store", headers: authHeaders, signal: AbortSignal.timeout(10_000) },
      );
      const assetPayload = (await safeJSON(assetRes)) as { asset?: Asset; error?: string } | null;
      if (assetRes.ok && assetPayload?.asset?.metadata) {
        const discoveredRaw = assetPayload.asset.metadata.discovered;
        const discoveredParsed = Number(discoveredRaw);
        if (Number.isFinite(discoveredParsed) && discoveredParsed >= 0) {
          discovered = discoveredParsed;
        }
      }
    }

    return NextResponse.json({
      collector,
      discovered,
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load collector status" },
      { status: 502 },
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
