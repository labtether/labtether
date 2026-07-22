import { NextResponse } from "next/server";

import { resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/api/v2/openapi.json`, {
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json({ error: "failed to load API specification" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {}, { status: response.status });
  } catch {
    return NextResponse.json({ error: "API specification endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
