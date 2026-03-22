import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function POST(
  request: Request,
  { params }: { params: Promise<{ collectorId: string }> },
) {
  const { collectorId } = await params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  if (!collectorId.trim()) {
    return NextResponse.json({ error: "collector id is required" }, { status: 400 });
  }

  try {
    const response = await fetch(
      `${base.api}/hub-collectors/${encodeURIComponent(collectorId)}/run`,
      {
        method: "POST",
        cache: "no-store",
        headers: authHeaders,
        signal: AbortSignal.timeout(15_000),
      },
    );
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to start collector run" }, { status: response.status });
    }
    return NextResponse.json(payload ?? { status: "started", collector_id: collectorId });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to start collector run" },
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

