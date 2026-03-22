import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs, upstreamErrorPayload } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/settings/managed-database/reveal`, {
      method: "POST",
      cache: "no-store",
      headers: authHeaders,
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to reveal managed database password"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "managed database settings endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
