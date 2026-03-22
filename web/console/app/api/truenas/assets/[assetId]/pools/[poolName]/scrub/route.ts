import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../../lib/backend";

export const dynamic = "force-dynamic";

type Params = { params: Promise<{ assetId: string; poolName: string }> };

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}

export async function POST(request: Request, { params }: Params) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId, poolName } = await params;
  const assetID = encodeURIComponent(assetId ?? "");
  const poolID = encodeURIComponent(poolName ?? "");
  try {
    const response = await fetch(`${base.api}/truenas/assets/${assetID}/pools/${poolID}/scrub`, {
      method: "POST",
      headers: authHeaders,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to start scrub" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to start scrub" },
      { status: 502 },
    );
  }
}
