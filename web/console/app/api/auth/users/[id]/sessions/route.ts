import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs, upstreamErrorPayload } from "../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type RouteContext = { params: Promise<{ id: string }> };

export async function GET(request: Request, { params }: RouteContext) {
  const routeParams = await params;
  const id = routeParams.id?.trim();
  if (!id) {
    return NextResponse.json({ error: "user id is required" }, { status: 400 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/auth/users/${encodeURIComponent(id)}/sessions`, {
      cache: "no-store",
      headers: authHeaders,
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to load sessions"),
        { status: response.status },
      );
    }

    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "users endpoint unavailable" }, { status: 502 });
  }
}

export async function DELETE(request: Request, { params }: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const routeParams = await params;
  const id = routeParams.id?.trim();
  if (!id) {
    return NextResponse.json({ error: "user id is required" }, { status: 400 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/auth/users/${encodeURIComponent(id)}/sessions`, {
      method: "DELETE",
      cache: "no-store",
      headers: authHeaders,
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to revoke sessions"),
        { status: response.status },
      );
    }

    return NextResponse.json(payload ?? {}, { status: response.status });
  } catch {
    return NextResponse.json({ error: "users endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
