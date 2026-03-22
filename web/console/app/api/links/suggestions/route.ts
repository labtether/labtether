import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/links/suggestions`, {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to list link suggestions" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to query link suggestions" },
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
