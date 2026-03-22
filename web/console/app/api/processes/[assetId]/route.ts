import { NextRequest, NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ assetId: string }>;
};

export async function GET(request: NextRequest, context: RouteContext) {
  const { assetId } = await context.params;
  const sort = request.nextUrl.searchParams.get("sort") || "cpu";
  const limit = request.nextUrl.searchParams.get("limit") || "25";

  try {
    const base = await resolvedBackendBaseURLs();
    const url = `${base.api}/processes/${encodeURIComponent(assetId)}?sort=${encodeURIComponent(sort)}&limit=${encodeURIComponent(limit)}`;
    const headers = backendAuthHeadersWithCookie(request);

    const res = await fetch(url, { headers, cache: "no-store" });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to fetch process list" },
      { status: 502 }
    );
  }
}

export async function POST(request: NextRequest, context: RouteContext) {
  const { assetId } = await context.params;

  try {
    const base = await resolvedBackendBaseURLs();
    const url = `${base.api}/processes/${encodeURIComponent(assetId)}/kill`;
    const headers = {
      ...backendAuthHeadersWithCookie(request),
      "Content-Type": "application/json",
    };

    const res = await fetch(url, {
      method: "POST",
      headers,
      body: await request.text(),
      cache: "no-store",
    });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to signal process" },
      { status: 502 }
    );
  }
}
