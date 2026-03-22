import { NextRequest, NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteParams = {
  params: Promise<{ id: string }>;
};

export async function PATCH(request: NextRequest, { params }: RouteParams) {
  return updateManualService(request, params);
}

export async function PUT(request: NextRequest, { params }: RouteParams) {
  return updateManualService(request, params);
}

export async function DELETE(request: NextRequest, { params }: RouteParams) {
  const { id } = await params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/api/v1/services/web/manual/${encodeURIComponent(id)}`, {
      method: "DELETE",
      cache: "no-store",
      headers: authHeaders,
    });
    if (response.status === 204) {
      return new NextResponse(null, { status: 204 });
    }
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to delete manual service" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "request failed" },
      { status: 502 }
    );
  }
}

async function updateManualService(request: NextRequest, paramsPromise: Promise<{ id: string }>) {
  const { id } = await paramsPromise;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  let body: unknown = {};
  try {
    body = await request.json();
  } catch {
    body = {};
  }

  try {
    const response = await fetch(`${base.api}/api/v1/services/web/manual/${encodeURIComponent(id)}`, {
      method: "PATCH",
      cache: "no-store",
      headers: {
        ...authHeaders,
        "content-type": "application/json",
      },
      body: JSON.stringify(body ?? {}),
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to update manual service" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "request failed" },
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
