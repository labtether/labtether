import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export async function GET(request: Request) {
  try {
    const url = new URL(request.url);
    const scope = url.searchParams.get("scope");

    const base = await resolvedBackendBaseURLs();
    const backendURL = new URL(`${base.api}/terminal/snippets`);
    if (scope) {
      backendURL.searchParams.set("scope", scope);
    }

    const response = await fetch(backendURL.toString(), {
      method: "GET",
      headers: {
        ...backendAuthHeadersWithCookie(request),
      },
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        { error: (payload as Record<string, unknown>)?.error || "failed to fetch snippets" },
        { status: response.status || 502 },
      );
    }

    return NextResponse.json(payload);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "snippets endpoint unavailable" },
      { status: 502 },
    );
  }
}

export async function POST(request: Request) {
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/snippets`, {
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
        { error: (payload as Record<string, unknown>)?.error || "failed to create snippet" },
        { status: response.status || 502 },
      );
    }

    return NextResponse.json(payload);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "snippets endpoint unavailable" },
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
