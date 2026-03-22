import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

type RouteParams = { params: Promise<{ id: string }> };

export async function GET(request: Request, props: RouteParams) {
  const { id } = await props.params;
  if (!id) {
    return NextResponse.json({ error: "snippet id is required" }, { status: 400 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/snippets/${encodeURIComponent(id)}`, {
      method: "GET",
      headers: {
        ...backendAuthHeadersWithCookie(request),
      },
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        { error: (payload as Record<string, unknown>)?.error || "failed to fetch snippet" },
        { status: response.status || 502 },
      );
    }

    return NextResponse.json(payload);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "snippet endpoint unavailable" },
      { status: 502 },
    );
  }
}

export async function PUT(request: Request, props: RouteParams) {
  const { id } = await props.params;
  if (!id) {
    return NextResponse.json({ error: "snippet id is required" }, { status: 400 });
  }

  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/snippets/${encodeURIComponent(id)}`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request),
      },
      body: JSON.stringify(body),
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        { error: (payload as Record<string, unknown>)?.error || "failed to update snippet" },
        { status: response.status || 502 },
      );
    }

    return NextResponse.json(payload);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "snippet endpoint unavailable" },
      { status: 502 },
    );
  }
}

export async function DELETE(request: Request, props: RouteParams) {
  const { id } = await props.params;
  if (!id) {
    return NextResponse.json({ error: "snippet id is required" }, { status: 400 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/snippets/${encodeURIComponent(id)}`, {
      method: "DELETE",
      headers: {
        ...backendAuthHeadersWithCookie(request),
      },
    });

    if (response.status === 204) {
      return new NextResponse(null, { status: 204 });
    }

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        { error: (payload as Record<string, unknown>)?.error || "failed to delete snippet" },
        { status: response.status || 502 },
      );
    }

    return NextResponse.json(payload);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "snippet endpoint unavailable" },
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
