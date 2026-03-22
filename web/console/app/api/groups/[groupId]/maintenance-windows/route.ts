import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ groupId: string }>;
};

export async function GET(request: Request, context: RouteContext) {
  const { groupId } = await context.params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const incoming = new URL(request.url);
  const backendURL = new URL(`${base.api}/groups/${encodeURIComponent(groupId)}/maintenance-windows`);

  for (const [key, value] of incoming.searchParams.entries()) {
    backendURL.searchParams.set(key, value);
  }

  try {
    const response = await fetch(backendURL.toString(), {
      cache: "no-store",
      headers: authHeaders
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to load maintenance windows" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to query backend maintenance windows endpoint" },
      { status: 502 }
    );
  }
}

export async function POST(request: Request, context: RouteContext) {
  const { groupId } = await context.params;
  let payload: unknown;
  try {
    payload = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/groups/${encodeURIComponent(groupId)}/maintenance-windows`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders
      },
      body: JSON.stringify(payload)
    });

    const responsePayload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(responsePayload ?? { error: "failed to create maintenance window" }, { status: response.status });
    }
    return NextResponse.json(responsePayload ?? {}, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to create maintenance window" },
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
