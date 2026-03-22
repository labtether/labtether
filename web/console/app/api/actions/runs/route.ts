import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  const incoming = new URL(request.url);
  const backendURL = new URL(`${base.api}/actions/runs`);
  for (const [key, value] of incoming.searchParams.entries()) {
    backendURL.searchParams.set(key, value);
  }

  try {
    const response = await fetch(backendURL.toString(), {
      cache: "no-store",
      headers: authHeaders
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to load action runs" }, { status: response.status });
    }

    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to query backend action runs" },
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
