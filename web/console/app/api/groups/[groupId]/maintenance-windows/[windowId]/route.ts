import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ groupId: string; windowId: string }>;
};

export async function GET(request: Request, context: RouteContext) {
  const { groupId, windowId } = await context.params;
  return proxyWindowRequest(request, "GET", groupId, windowId);
}

export async function PUT(request: Request, context: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const { groupId, windowId } = await context.params;
  let payload: unknown;
  try {
    payload = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }
  return proxyWindowRequest(request, "PUT", groupId, windowId, payload);
}

export async function PATCH(request: Request, context: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const { groupId, windowId } = await context.params;
  let payload: unknown;
  try {
    payload = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }
  return proxyWindowRequest(request, "PATCH", groupId, windowId, payload);
}

export async function DELETE(request: Request, context: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const { groupId, windowId } = await context.params;
  return proxyWindowRequest(request, "DELETE", groupId, windowId);
}

async function proxyWindowRequest(request: Request, method: "GET" | "PUT" | "PATCH" | "DELETE", groupId: string, windowId: string, payload?: unknown) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(
      `${base.api}/groups/${encodeURIComponent(groupId)}/maintenance-windows/${encodeURIComponent(windowId)}`,
      {
        method,
        headers:
          method === "PUT" || method === "PATCH"
            ? {
                "Content-Type": "application/json",
                ...authHeaders
              }
            : authHeaders,
        body: method === "PUT" || method === "PATCH" ? JSON.stringify(payload ?? {}) : undefined
      }
    );

    const responsePayload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(responsePayload ?? { error: "maintenance window request failed" }, { status: response.status });
    }
    return NextResponse.json(responsePayload ?? {}, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "maintenance window request failed" },
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
