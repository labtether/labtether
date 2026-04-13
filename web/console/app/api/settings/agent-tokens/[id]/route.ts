import { NextResponse } from "next/server";
import { backendBaseURLs, backendAuthHeadersWithCookie } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export async function DELETE(request: Request, { params }: { params: Promise<{ id: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const { id } = await params;
    const base = backendBaseURLs();
    const res = await fetch(`${base.api}/settings/agent-tokens/${encodeURIComponent(id)}`, {
      method: "DELETE",
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "failed to revoke token" }, { status: 500 });
  }
}
