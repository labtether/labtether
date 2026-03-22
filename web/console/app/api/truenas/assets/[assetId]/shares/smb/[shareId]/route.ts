import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../../lib/backend";

export const dynamic = "force-dynamic";

type Params = { params: Promise<{ assetId: string; shareId: string }> };

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}

export async function PUT(request: Request, { params }: Params) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId, shareId } = await params;
  const assetID = encodeURIComponent(assetId ?? "");
  const id = encodeURIComponent(shareId ?? "");
  const body = await request.text();
  try {
    const response = await fetch(`${base.api}/truenas/assets/${assetID}/shares/smb/${id}`, {
      method: "PUT",
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to update SMB share" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to update SMB share" },
      { status: 502 },
    );
  }
}

export async function DELETE(request: Request, { params }: Params) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId, shareId } = await params;
  const assetID = encodeURIComponent(assetId ?? "");
  const id = encodeURIComponent(shareId ?? "");
  try {
    const response = await fetch(`${base.api}/truenas/assets/${assetID}/shares/smb/${id}`, {
      method: "DELETE",
      headers: authHeaders,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to delete SMB share" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to delete SMB share" },
      { status: 502 },
    );
  }
}
