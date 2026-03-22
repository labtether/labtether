import { NextRequest, NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ assetId: string }>;
};

export async function GET(request: NextRequest, context: RouteContext) {
  const { assetId } = await context.params;

  try {
    const base = await resolvedBackendBaseURLs();
    const url = `${base.api}/disks/${encodeURIComponent(assetId)}`;
    const headers = backendAuthHeadersWithCookie(request);

    const res = await fetch(url, { headers, cache: "no-store" });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to fetch disk list" },
      { status: 502 }
    );
  }
}
