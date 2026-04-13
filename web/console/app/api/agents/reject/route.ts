import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, backendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const body = (await request.json()) as { asset_id?: string };
    const base = backendBaseURLs();
    const response = await fetch(`${base.api}/api/v1/agents/reject`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request),
      },
      body: JSON.stringify(body),
    });

    if (!response.ok) {
      return NextResponse.json({ error: "reject failed" }, { status: response.status });
    }

    const data = (await response.json()) as { status?: string };
    return NextResponse.json(data);
  } catch {
    return NextResponse.json({ error: "reject failed" }, { status: 500 });
  }
}
