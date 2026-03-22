import { NextRequest, NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ assetId: string; action: string }>;
};

export async function POST(request: NextRequest, context: RouteContext) {
  const { assetId, action } = await context.params;

  try {
    const body = await request.json().catch(() => ({}));
    const base = await resolvedBackendBaseURLs();
    const url = `${base.api}/network/${encodeURIComponent(assetId)}/${encodeURIComponent(action)}`;
    const headers = backendAuthHeadersWithCookie(request);
    const response = await fetch(url, {
      method: "POST",
      headers: { ...headers, "Content-Type": "application/json" },
      body: JSON.stringify(body ?? {}),
      cache: "no-store",
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? {}, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "network action failed" },
      { status: 502 }
    );
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
