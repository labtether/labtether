import { NextRequest, NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs, upstreamErrorPayload } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: NextRequest) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/api/v1/services/web/never-group-rules`, {
      cache: "no-store",
      headers: authHeaders,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to load never-group rules"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "never-group rules endpoint unavailable" }, { status: 502 });
  }
}

export async function POST(request: NextRequest) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  const body = await safeRequestJSON(request);
  if (body === null) {
    return NextResponse.json({ error: "invalid request body" }, { status: 400 });
  }

  try {
    const response = await fetch(`${base.api}/api/v1/services/web/never-group-rules`, {
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
        upstreamErrorPayload(response.status, payload, "failed to add never-group rule"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "never-group rules endpoint unavailable" }, { status: 502 });
  }
}

export async function DELETE(request: NextRequest) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  const ruleId = request.nextUrl.searchParams.get("id") ?? "";
  if (!ruleId.trim()) {
    return NextResponse.json({ error: "missing rule id" }, { status: 400 });
  }

  try {
    const response = await fetch(`${base.api}/api/v1/services/web/never-group-rules?id=${encodeURIComponent(ruleId)}`, {
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
        upstreamErrorPayload(response.status, payload, "failed to delete never-group rule"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "never-group rules endpoint unavailable" }, { status: 502 });
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
