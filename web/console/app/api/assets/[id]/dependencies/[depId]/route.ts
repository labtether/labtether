import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ id: string; depId: string }>;
};

export async function DELETE(request: Request, context: RouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const { id, depId } = await context.params;
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(
      `${base.api}/assets/${encodeURIComponent(id)}/dependencies/${encodeURIComponent(depId)}`,
      {
        method: "DELETE",
        cache: "no-store",
        headers: { ...backendAuthHeadersWithCookie(request) },
      },
    );
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to delete dependency" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to delete dependency" },
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
