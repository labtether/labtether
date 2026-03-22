import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export async function GET(request: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/assets/${encodeURIComponent(id)}/displays`, {
      cache: "no-store",
      headers: {
        ...backendAuthHeadersWithCookie(request),
      },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { displays: [] }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "display query failed" },
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
