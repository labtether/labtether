import { NextResponse } from "next/server";

import {
  backendAuthHeadersWithCookie,
  backendBaseURLs,
  resolvedBackendBaseURLs,
} from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ groupId: string }>;
};

export async function GET(request: Request, context: RouteContext) {
  try {
    const { groupId } = await context.params;
    const response = await fetchGroupBackend(
      `/groups/${encodeURIComponent(groupId)}`,
      {
        cache: "no-store",
        headers: { ...backendAuthHeadersWithCookie(request) },
      },
    );
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to load group" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load group" },
      { status: 502 },
    );
  }
}

export async function PUT(request: Request, context: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  return proxyMutation(request, context, "PUT");
}

export async function PATCH(request: Request, context: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  return proxyMutation(request, context, "PATCH");
}

export async function DELETE(request: Request, context: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const { groupId } = await context.params;
    const response = await fetchGroupBackend(
      `/groups/${encodeURIComponent(groupId)}`,
      {
        method: "DELETE",
        cache: "no-store",
        headers: { ...backendAuthHeadersWithCookie(request) },
      },
    );
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to delete group" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to delete group" },
      { status: 502 },
    );
  }
}

async function proxyMutation(request: Request, context: RouteContext, method: "PUT" | "PATCH") {
  try {
    const { groupId } = await context.params;
    const payload = await request.json();
    const response = await fetchGroupBackend(
      `/groups/${encodeURIComponent(groupId)}`,
      {
        method,
        cache: "no-store",
        headers: {
          ...backendAuthHeadersWithCookie(request),
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
      },
    );
    const responsePayload = await safeJSON(response);
    return NextResponse.json(responsePayload ?? { error: "failed to update group" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to update group" },
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

async function fetchGroupBackend(path: string, init: RequestInit): Promise<Response> {
  const resolved = await resolvedBackendBaseURLs();
  const baseline = backendBaseURLs();
  const origins = [resolved.api, baseline.api].filter(
    (value, index, items) => value && items.indexOf(value) === index,
  );

  let lastError: unknown = null;
  for (const origin of origins) {
    try {
      return await fetch(new URL(path, `${origin}/`).toString(), init);
    } catch (error) {
      lastError = error;
    }
  }

  throw lastError instanceof Error ? lastError : new Error("failed to reach backend group endpoint");
}
