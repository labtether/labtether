import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ id: string; protocol: string }>;
};

export async function PUT(request: Request, context: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const { id, protocol } = await context.params;
    const body = await request.json();
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(
      `${base.api}/assets/${encodeURIComponent(id)}/protocols/${encodeURIComponent(protocol)}`,
      {
        method: "PUT",
        cache: "no-store",
        headers: {
          ...backendAuthHeadersWithCookie(request),
          "Content-Type": "application/json",
        },
        body: JSON.stringify(body),
      },
    );
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to update protocol" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to update protocol" },
      { status: 502 },
    );
  }
}

export async function DELETE(request: Request, context: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const { id, protocol } = await context.params;
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(
      `${base.api}/assets/${encodeURIComponent(id)}/protocols/${encodeURIComponent(protocol)}`,
      {
        method: "DELETE",
        cache: "no-store",
        headers: { ...backendAuthHeadersWithCookie(request) },
      },
    );
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to delete protocol" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to delete protocol" },
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
