import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: Request, context: { params: Promise<{ id: string }> }) {
  const { id } = await context.params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/api/v1/agents/${encodeURIComponent(id)}/settings/history`, {
      cache: "no-store",
      headers: authHeaders
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to load agent settings history" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load agent settings history" },
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
