import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { readBoundedRequestBody, RequestBodyTooLargeError } from "../../../../lib/boundedBody";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

const maxCreateBodyBytes = 64 * 1024;

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/persistent-sessions`, {
      method: "GET",
      headers: {
        ...backendAuthHeadersWithCookie(request),
      },
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        { error: (payload as Record<string, unknown>)?.error || "failed to fetch persistent sessions" },
        { status: response.status || 502 },
      );
    }

    return NextResponse.json(payload);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "persistent sessions endpoint unavailable" },
      { status: 502 },
    );
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  let rawBody: ArrayBuffer;
  try {
    rawBody = await readBoundedRequestBody(request, maxCreateBodyBytes);
  } catch (error) {
    if (error instanceof RequestBodyTooLargeError) {
      return NextResponse.json({ error: "request body too large" }, { status: 413 });
    }
    return NextResponse.json({ error: "invalid request body" }, { status: 400 });
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(new TextDecoder().decode(rawBody));
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }
  const values = parsed as Record<string, unknown>;
  const target = typeof values.target === "string" ? values.target.trim() : "";
  if (!target) {
    return NextResponse.json({ error: "target is required" }, { status: 400 });
  }

  const body: Record<string, string> = { target };
  if (typeof values.title === "string") body.title = values.title.trim();
  if (typeof values.bookmark_id === "string") body.bookmark_id = values.bookmark_id.trim();

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/persistent-sessions`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request),
      },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(15_000),
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        { error: (payload as Record<string, unknown> | null)?.error || "failed to create persistent session" },
        { status: response.status || 502 },
      );
    }
    return NextResponse.json(payload, { status: response.status });
  } catch {
    return NextResponse.json({ error: "persistent sessions endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
