import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../lib/backend";

export const dynamic = "force-dynamic";

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export async function DELETE(request: Request, { params }: { params: Promise<{ id: string; assetId: string }> }) {
  try {
    const { id, assetId } = await params;
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/composites/${id}/members/${assetId}`, {
      method: "DELETE",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? {}, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to remove composite member" },
      { status: 502 },
    );
  }
}
