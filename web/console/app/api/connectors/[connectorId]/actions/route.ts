import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ connectorId: string }>;
};

export async function GET(request: Request, context: RouteContext) {
  const { connectorId } = await context.params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  const incoming = new URL(request.url);
  const backendURL = new URL(`${base.api}/connectors/${encodeURIComponent(connectorId)}/actions`);
  const limit = incoming.searchParams.get("limit");
  if (limit) {
    backendURL.searchParams.set("limit", limit);
  }

  try {
    const response = await fetch(backendURL.toString(), {
      cache: "no-store",
      headers: authHeaders
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to load connector actions" }, { status: response.status });
    }

    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to query connector actions" },
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
