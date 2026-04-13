import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../../lib/proxyAuth";

export async function POST(request: Request, { params }: { params: Promise<{ id: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const { id } = await params;
    const base = await resolvedBackendBaseURLs();
    const res = await fetch(
      `${base.api}/assets/${encodeURIComponent(id)}/desktop/credentials/retrieve`,
      {
        method: "POST",
        cache: "no-store",
        headers: { "Content-Type": "application/json", ...backendAuthHeadersWithCookie(request) },
      }
    );
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "failed to retrieve desktop credentials" }, { status: 500 });
  }
}
