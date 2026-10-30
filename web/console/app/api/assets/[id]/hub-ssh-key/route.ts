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
    const response = await fetch(`${base.api}/assets/${encodeURIComponent(id)}/hub-ssh-key`, {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to load hub SSH key setting" }, { status: response.status });
  } catch {
    return NextResponse.json({ error: "failed to load hub SSH key setting" }, { status: 502 });
  }
}

export async function PUT(request: Request, context: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const { id } = await context.params;
    const body = await request.text();
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/assets/${encodeURIComponent(id)}/hub-ssh-key`, {
      method: "PUT",
      cache: "no-store",
      headers: {
        ...backendAuthHeadersWithCookie(request),
        "Content-Type": "application/json",
      },
      body,
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to save hub SSH key setting" }, { status: response.status });
  } catch {
    return NextResponse.json({ error: "failed to save hub SSH key setting" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
