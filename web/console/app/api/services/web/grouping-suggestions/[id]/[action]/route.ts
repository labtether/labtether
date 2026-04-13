import { NextRequest, NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string; action: string }> }
) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { id, action } = await params;

  if (action !== "accept" && action !== "deny") {
    return NextResponse.json({ error: "invalid action" }, { status: 400 });
  }

  try {
    const response = await fetch(
      `${base.api}/api/v1/services/web/grouping-suggestions/${encodeURIComponent(id)}/${encodeURIComponent(action)}`,
      {
        method: "POST",
        cache: "no-store",
        headers: authHeaders,
      }
    );
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: `failed to ${action} suggestion` }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "request failed" },
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
