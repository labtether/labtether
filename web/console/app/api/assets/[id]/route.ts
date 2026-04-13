import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

export async function DELETE(request: Request, { params }: { params: Promise<{ id: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const { id } = await params;
    const base = await resolvedBackendBaseURLs();
    const res = await fetch(`${base.api}/assets/${encodeURIComponent(id)}`, {
      method: "DELETE",
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "failed to delete asset" }, { status: 500 });
  }
}

export async function PATCH(request: Request, { params }: { params: Promise<{ id: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const { id } = await params;
    const body = await request.text();
    const base = await resolvedBackendBaseURLs();
    const res = await fetch(`${base.api}/assets/${encodeURIComponent(id)}`, {
      method: "PATCH",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request),
      },
      body,
    });
    const data = await safeJSON(res);
    return NextResponse.json(data ?? { error: "failed to update asset" }, { status: res.status });
  } catch {
    return NextResponse.json({ error: "failed to update asset" }, { status: 500 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
