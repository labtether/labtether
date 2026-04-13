import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type Params = { params: Promise<{ assetId: string; datasetId: string }> };

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}

export async function DELETE(request: Request, { params }: Params) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId, datasetId } = await params;
  const assetID = encodeURIComponent(assetId ?? "");
  const dsID = encodeURIComponent(datasetId ?? "");
  try {
    const response = await fetch(`${base.api}/truenas/assets/${assetID}/datasets/${dsID}`, {
      method: "DELETE",
      headers: authHeaders,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to delete dataset" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to delete dataset" },
      { status: 502 },
    );
  }
}
