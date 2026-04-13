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

  // Only forward known fields to the backend — drop anything unexpected.
  const cleanBody = JSON.stringify({
    base_url: typeof parsed.base_url === "string" ? parsed.base_url : "",
    api_key: typeof parsed.api_key === "string" ? parsed.api_key : undefined,
    credential_id: typeof parsed.credential_id === "string" ? parsed.credential_id : undefined,
    skip_verify: typeof parsed.skip_verify === "boolean" ? parsed.skip_verify : false,
  });
  const secretCandidates = [typeof parsed.api_key === "string" ? parsed.api_key : ""];

  try {
    const response = await fetch(`${base.api}/connectors/truenas/test`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders
      },
      body: cleanBody
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        sanitizeTestRouteError(payload, "failed to test truenas connection", secretCandidates),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      sanitizeTestRouteError(error instanceof Error ? error.message : "", "failed to test truenas connection", secretCandidates),
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
