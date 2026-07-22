import { NextRequest, NextResponse } from "next/server";
import { backendBaseURLs, backendAuthHeadersWithCookie } from "../../../../lib/backend";
import { publicHubOriginHeaders } from "../../../../lib/publicHubProxy";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

const noStoreHeaders = { "Cache-Control": "no-store" };

export async function GET(request: NextRequest) {
  try {
    const base = backendBaseURLs();
    const headers = publicHubOriginHeaders(request);
    for (const [name, value] of Object.entries(backendAuthHeadersWithCookie(request))) {
      headers.set(name, value);
    }
    const res = await fetch(`${base.api}/settings/enrollment`, {
      cache: "no-store",
      headers,
    });
    if (!res.ok) {
      const payload = (await res.json().catch(() => null)) as { error?: string } | null;
      return NextResponse.json(payload ?? { error: "failed to fetch enrollment tokens" }, {
        status: res.status,
        headers: noStoreHeaders,
      });
    }
    const data = await res.json();
    return NextResponse.json(data, { headers: noStoreHeaders });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to fetch enrollment tokens" },
      { status: 502, headers: noStoreHeaders }
    );
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const base = backendBaseURLs();
    const body = await request.json();
    const res = await fetch(`${base.api}/settings/enrollment`, {
      method: "POST",
      cache: "no-store",
      headers: { "Content-Type": "application/json", ...backendAuthHeadersWithCookie(request) },
      body: JSON.stringify(body),
    });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "failed to create token" }, { status: 500 });
  }
}
