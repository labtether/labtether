import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const url = new URL(request.url);
  const backendURL = new URL(`${base.api}/queue/dead-letters`);
  backendURL.search = url.search;

  try {
    const response = await fetch(backendURL, {
      cache: "no-store",
      headers: {
        ...backendAuthHeadersWithCookie(request)
      }
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to query dead-letter events" }, { status: response.status });
    }
    return NextResponse.json(payload ?? { events: [] });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "dead-letter endpoint unavailable" },
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
