import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export async function POST(request: Request, { params }: { params: Promise<{ sessionId: string }> }) {
  try {
    const { sessionId } = await params;
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/recordings/${encodeURIComponent(sessionId)}`, {
      method: "POST",
      cache: "no-store",
      headers: {
        ...backendAuthHeadersWithCookie(request),
      },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to stop recording" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to stop recording" },
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
