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

  let parsed: Record<string, unknown> = {};
  try {
    parsed = (await request.json()) as Record<string, unknown>;
  } catch {
    parsed = {};
  }

  const cleanBody = JSON.stringify({
    base_url: typeof parsed.base_url === "string" ? parsed.base_url : "",
    token_id: typeof parsed.token_id === "string" ? parsed.token_id : "",
    token_secret: typeof parsed.token_secret === "string" ? parsed.token_secret : "",
    credential_id: typeof parsed.credential_id === "string" ? parsed.credential_id : "",
    skip_verify: typeof parsed.skip_verify === "boolean" ? parsed.skip_verify : false,
    ca_pem: typeof parsed.ca_pem === "string" ? parsed.ca_pem : "",
  });
  const secretCandidates = [typeof parsed.token_secret === "string" ? parsed.token_secret : ""];

  try {
    const response = await fetch(`${base.api}/connectors/pbs/test`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders,
      },
      signal: AbortSignal.timeout(25_000),
      body: cleanBody,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        sanitizeTestRouteError(payload, "failed to test pbs connection", secretCandidates),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      sanitizeTestRouteError(error instanceof Error ? error.message : "", "failed to test pbs connection", secretCandidates),
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
