import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";

type RouteParams = { params: Promise<{ id: string }> };

export async function POST(request: Request, props: RouteParams) {
  const { id } = await props.params;
  if (!id) {
    return NextResponse.json({ error: "persistent session id is required" }, { status: 400 });
  }

  let body: unknown;
  try {
    body = await request.json();
  } catch {
    body = {};
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/persistent-sessions/${encodeURIComponent(id)}/attach`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request),
      },
      body: JSON.stringify(body),
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        { error: (payload as Record<string, unknown>)?.error || "failed to attach persistent session" },
        { status: response.status || 502 },
      );
    }

    return NextResponse.json(payload, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "persistent session attach endpoint unavailable" },
      { status: 502 },
    );
  }
}

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
