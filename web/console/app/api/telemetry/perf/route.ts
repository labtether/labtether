import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const payload = await safeJSON(request);
  if (!payload || typeof payload !== "object") {
    return NextResponse.json({ error: "invalid telemetry payload" }, { status: 400 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  authHeaders["Content-Type"] = "application/json";

  try {
    const response = await fetch(`${base.api}/telemetry/frontend/perf`, {
      method: "POST",
      cache: "no-store",
      headers: authHeaders,
      body: JSON.stringify(payload),
    });

    const body = await safeResponseJSON(response);
    if (!response.ok) {
      return NextResponse.json(body ?? { error: "failed to record telemetry" }, { status: response.status });
    }
    return NextResponse.json(body ?? { accepted: true }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to forward telemetry" },
      { status: 502 },
    );
  }
}

async function safeJSON(request: Request): Promise<unknown | null> {
  try {
    return await request.json();
  } catch {
    return null;
  }
}

async function safeResponseJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

