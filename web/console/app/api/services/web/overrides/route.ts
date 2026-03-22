import { NextRequest, NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs, upstreamErrorPayload } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

export async function GET(request: NextRequest) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const hostParam = request.nextUrl.searchParams.get("host") ?? "";
    const host = normalizeSelectorValue(hostParam);
    if (host === null) {
      return NextResponse.json({ error: "invalid host selector" }, { status: 400 });
    }
    const qs = host ? `?host=${encodeURIComponent(host)}` : "";
    const response = await fetch(`${base.api}/api/v1/services/web/overrides${qs}`, {
      cache: "no-store",
      headers: authHeaders,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to load service overrides"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "service override endpoint unavailable" }, { status: 502 });
  }
}

export async function POST(request: NextRequest) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  const body = await safeRequestJSON(request);
  if (body === null) {
    return NextResponse.json({ error: "invalid request body" }, { status: 400 });
  }

  try {
    const response = await fetch(`${base.api}/api/v1/services/web/overrides`, {
      method: "POST",
      cache: "no-store",
      headers: {
        ...authHeaders,
        "content-type": "application/json",
      },
      body: JSON.stringify(body ?? {}),
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to save service override"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "service override endpoint unavailable" }, { status: 502 });
  }
}

export async function DELETE(request: NextRequest) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  const host = normalizeSelectorValue(request.nextUrl.searchParams.get("host") ?? "");
  if (host === null) {
    return NextResponse.json({ error: "invalid host selector" }, { status: 400 });
  }
  const serviceID = normalizeSelectorValue(request.nextUrl.searchParams.get("service_id") ?? "");
  if (serviceID === null) {
    return NextResponse.json({ error: "invalid service selector" }, { status: 400 });
  }
  const params = new URLSearchParams();
  if (host) params.set("host", host);
  if (serviceID) params.set("service_id", serviceID);
  const qs = params.toString() ? `?${params.toString()}` : "";

  try {
    const response = await fetch(`${base.api}/api/v1/services/web/overrides${qs}`, {
      method: "DELETE",
      cache: "no-store",
      headers: authHeaders,
    });
    if (response.status === 204) {
      return new NextResponse(null, { status: 204 });
    }
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to delete service override"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "service override endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

async function safeRequestJSON(request: Request): Promise<Record<string, unknown> | null> {
  try {
    const payload = await request.json();
    if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
      return null;
    }
    return payload as Record<string, unknown>;
  } catch {
    return null;
  }
}

function normalizeSelectorValue(raw: string): string | null {
  const value = raw.trim();
  if (value === "") {
    return "";
  }
  if (value.length > 160) {
    return null;
  }
  if (!/^[a-zA-Z0-9][a-zA-Z0-9._:-]*$/.test(value)) {
    return null;
  }
  return value;
}
