import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/persistent-sessions`, {
      method: "GET",
      headers: {
        ...backendAuthHeadersWithCookie(request),
      },
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        { error: (payload as Record<string, unknown>)?.error || "failed to fetch persistent sessions" },
        { status: response.status || 502 },
      );
    }

    return NextResponse.json(payload);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "persistent sessions endpoint unavailable" },
      { status: 502 },
    );
  }
}

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
