import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { sanitizeTestRouteError } from "../../testErrorSanitizer";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  let parsed: Record<string, unknown> = {};
  try {
    parsed = (await request.json()) as Record<string, unknown>;
  } catch {
    parsed = {};
  }

  const url = typeof parsed.url === "string" ? parsed.url : "";
  const username = typeof parsed.username === "string" ? parsed.username : "";
  const password = typeof parsed.password === "string" ? parsed.password : "";

  const cleanBody = JSON.stringify({ url, username, password });
  const secretCandidates = [password];

  try {
    const response = await fetch(`${base.api}/settings/prometheus/test-connection`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders,
      },
      body: cleanBody,
      signal: AbortSignal.timeout(20_000),
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        sanitizeTestRouteError(payload, "failed to test prometheus remote_write connection", secretCandidates),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      sanitizeTestRouteError(
        error instanceof Error ? error.message : "",
        "failed to test prometheus remote_write connection",
        secretCandidates,
      ),
      { status: 502 },
    );
  }
}
