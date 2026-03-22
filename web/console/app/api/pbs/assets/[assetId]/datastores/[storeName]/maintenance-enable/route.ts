import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../../lib/backend";

export const dynamic = "force-dynamic";

type Params = { params: Promise<{ assetId: string; storeName: string }> };

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}

export async function POST(request: Request, { params }: Params) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId, storeName } = await params;
  const assetID = encodeURIComponent(assetId ?? "");
  const storeID = encodeURIComponent(storeName ?? "");
  try {
    const response = await fetch(
      `${base.api}/pbs/assets/${assetID}/datastores/${storeID}/maintenance-enable`,
      {
        method: "POST",
        headers: authHeaders,
        signal: AbortSignal.timeout(20_000),
      },
    );
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to enable maintenance mode" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "failed to enable maintenance mode" }, { status: 502 });
  }
}
