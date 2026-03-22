import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../../lib/backend";

export const dynamic = "force-dynamic";

type Params = { params: Promise<{ assetId: string; diskName: string }> };

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}

export async function POST(request: Request, { params }: Params) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId, diskName } = await params;
  const assetID = encodeURIComponent(assetId ?? "");
  const disk = encodeURIComponent(diskName ?? "");
  const body = await request.text();
  try {
    const response = await fetch(
      `${base.api}/truenas/assets/${assetID}/disks/${disk}/smart-test`,
      {
        method: "POST",
        headers: { ...authHeaders, "Content-Type": "application/json" },
        body,
      },
    );
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to run SMART test" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to run SMART test" },
      { status: 502 },
    );
  }
}
