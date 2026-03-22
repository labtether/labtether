import { NextResponse } from "next/server";

import {
  backendAuthHeadersWithCookie,
  resolvedBackendBaseURLs,
  upstreamErrorPayload,
} from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/settings/tailscale/serve`, {
      cache: "no-store",
      headers: authHeaders,
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to load tailscale https status"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "tailscale https status endpoint unavailable" }, { status: 502 });
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const payload = await safeJSON(request);
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return NextResponse.json({ error: "invalid tailscale https request" }, { status: 400 });
  }

  try {
    const response = await fetch(`${base.api}/settings/tailscale/serve`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders,
      },
      body: JSON.stringify(payload),
    });

    const responsePayload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, responsePayload, "failed to update tailscale https"),
        { status: response.status },
      );
    }
    return NextResponse.json(responsePayload ?? {});
  } catch {
    return NextResponse.json({ error: "tailscale https endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response | Request): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
