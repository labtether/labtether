import { NextResponse } from "next/server";
import { backendBaseURLs, backendAuthHeadersWithCookie } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export async function DELETE(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const base = backendBaseURLs();
    const res = await fetch(`${base.api}/settings/tokens/cleanup`, {
      method: "DELETE",
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "failed to clean up tokens" }, { status: 500 });
  }
}
