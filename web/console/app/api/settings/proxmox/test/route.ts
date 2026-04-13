import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";
import { sanitizeTestRouteError } from "../../testErrorSanitizer";

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  let body = "";
  let parsed: Record<string, unknown> = {};
  try {
    body = await request.text();
    if (body.length > 16384) {
      return NextResponse.json({ error: "Request body too large" }, { status: 413 });
    }
    // Validate it's actually JSON
    if (body) {
      const decoded = JSON.parse(body);
      if (typeof decoded === "object" && decoded !== null) {
        parsed = decoded as Record<string, unknown>;
      }
    }
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }
  const secretCandidates = [
    typeof parsed.token_secret === "string" ? parsed.token_secret : "",
    typeof parsed.password === "string" ? parsed.password : "",
  ];

  try {
    const response = await fetch(`${base.api}/connectors/proxmox/test`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders
      },
      signal: AbortSignal.timeout(20_000),
      body
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        sanitizeTestRouteError(payload, "failed to test proxmox connection", secretCandidates),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      sanitizeTestRouteError(error instanceof Error ? error.message : "", "failed to test proxmox connection", secretCandidates),
      { status: 502 }
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
