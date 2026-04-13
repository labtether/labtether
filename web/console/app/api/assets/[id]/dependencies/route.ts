import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function GET(request: Request, context: RouteContext) {
  try {
    const { id } = await context.params;
    const base = await resolvedBackendBaseURLs();
    const incoming = new URL(request.url);
    const backendURL = new URL(`${base.api}/assets/${encodeURIComponent(id)}/dependencies`);
    for (const [key, value] of incoming.searchParams.entries()) {
      backendURL.searchParams.set(key, value);
    }

    const response = await fetch(backendURL.toString(), {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to load dependencies" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load dependencies" },
      { status: 502 },
    );
  }
}

export async function POST(request: Request, context: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const { id } = await context.params;
    const body = await request.json();
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/assets/${encodeURIComponent(id)}/dependencies`, {
      method: "POST",
      cache: "no-store",
      headers: {
        ...backendAuthHeadersWithCookie(request),
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to create dependency" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to create dependency" },
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
