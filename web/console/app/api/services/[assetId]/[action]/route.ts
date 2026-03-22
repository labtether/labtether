import { NextRequest, NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteContext = { params: Promise<{ assetId: string; action: string }> };

export async function POST(request: NextRequest, context: RouteContext) {
  const { assetId, action } = await context.params;
  try {
    const body = await request.json();
    const base = await resolvedBackendBaseURLs();
    const url = `${base.api}/services/${encodeURIComponent(assetId)}/${encodeURIComponent(action)}`;
    const headers = backendAuthHeadersWithCookie(request);
    const res = await fetch(url, {
      method: "POST",
      headers: { ...headers, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "service action failed" },
      { status: 502 }
    );
  }
}
