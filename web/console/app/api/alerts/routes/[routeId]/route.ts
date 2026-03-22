import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

function backendPath(baseURL: string, routeId: string): string {
  return `${baseURL}/alerts/routes/${encodeURIComponent(routeId)}`;
}

export async function GET(request: Request, context: { params: Promise<{ routeId: string }> }) {
  const { routeId } = await context.params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(backendPath(base.api, routeId), { cache: "no-store", headers: authHeaders });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to load alert route" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "backend error" },
      { status: 502 },
    );
  }
}

export async function PATCH(request: Request, context: { params: Promise<{ routeId: string }> }) {
  return mutateRoute(request, context, "PATCH");
}

export async function PUT(request: Request, context: { params: Promise<{ routeId: string }> }) {
  return mutateRoute(request, context, "PUT");
}

export async function DELETE(request: Request, context: { params: Promise<{ routeId: string }> }) {
  const { routeId } = await context.params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(backendPath(base.api, routeId), {
      method: "DELETE",
      headers: authHeaders,
      cache: "no-store",
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to delete alert route" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "backend error" },
      { status: 502 },
    );
  }
}

async function mutateRoute(
  request: Request,
  context: { params: Promise<{ routeId: string }> },
  method: "PATCH" | "PUT",
) {
  const { routeId } = await context.params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const body = await request.json();
    const response = await fetch(backendPath(base.api, routeId), {
      method,
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to update alert route" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "backend error" },
      { status: 502 },
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
