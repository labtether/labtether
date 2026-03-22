import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../../lib/backend";

export const dynamic = "force-dynamic";

type Params = { params: Promise<{ assetId: string; snapshotId: string; action: string }> };

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}

const ALLOWED_ACTIONS = new Set(["rollback", "clone"]);

export async function POST(request: Request, { params }: Params) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId, snapshotId, action } = await params;

  if (!ALLOWED_ACTIONS.has(action)) {
    return NextResponse.json({ error: "invalid action" }, { status: 400 });
  }

  const assetID = encodeURIComponent(assetId ?? "");
  const snapID = encodeURIComponent(snapshotId ?? "");
  const act = encodeURIComponent(action);
  try {
    const response = await fetch(
      `${base.api}/truenas/assets/${assetID}/snapshots/${snapID}/${act}`,
      {
        method: "POST",
        headers: authHeaders,
      },
    );
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: `failed to ${action} snapshot` }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : `failed to ${action} snapshot` },
      { status: 502 },
    );
  }
}
