import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";

type RouteParams = { params: Promise<{ id: string }> };

export async function GET(request: Request, props: RouteParams) {
  const { id } = await props.params;
  if (!id) {
    return NextResponse.json({ error: "session id is required" }, { status: 400 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/sessions/${encodeURIComponent(id)}/scrollback`, {
      method: "GET",
      headers: {
        ...backendAuthHeadersWithCookie(request),
      },
    });

    if (!response.ok) {
      let errorMessage = "failed to fetch scrollback";
      try {
        const payload = await response.json() as Record<string, unknown>;
        if (typeof payload?.error === "string" && payload.error) {
          errorMessage = payload.error;
        }
      } catch {
        // ignore parse errors
      }
      return NextResponse.json({ error: errorMessage }, { status: response.status || 502 });
    }

    const buffer = await response.arrayBuffer();
    return new NextResponse(buffer, {
      status: 200,
      headers: {
        "Content-Type": "application/octet-stream",
      },
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "scrollback endpoint unavailable" },
      { status: 502 },
    );
  }
}
