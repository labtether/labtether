import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";

export const dynamic = "force-dynamic";

type Params = {
  params: Promise<{
    assetId: string;
  }>;
};

export async function GET(request: Request, { params }: Params) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId } = await params;
  const assetID = encodeURIComponent(assetId ?? "");

  const incomingURL = new URL(request.url);
  const limit = incomingURL.searchParams.get("limit")?.trim() || "100";
  const window = incomingURL.searchParams.get("window")?.trim() || "6h";
  const query = new URLSearchParams({ limit, window });

  try {
    const response = await fetch(`${base.api}/truenas/assets/${assetID}/events?${query.toString()}`, {
      cache: "no-store",
      headers: authHeaders,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to load truenas events" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load truenas events" },
      { status: 502 },
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
