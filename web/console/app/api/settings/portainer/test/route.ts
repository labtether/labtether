import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { sanitizeTestRouteError } from "../../testErrorSanitizer";

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
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
    auth_method: typeof parsed.auth_method === "string" ? parsed.auth_method : "api_key",
    token_id: typeof parsed.token_id === "string" ? parsed.token_id : "",
    token_secret: typeof parsed.token_secret === "string" ? parsed.token_secret : "",
    credential_id: typeof parsed.credential_id === "string" ? parsed.credential_id : "",
    username: typeof parsed.username === "string" ? parsed.username : "",
    password: typeof parsed.password === "string" ? parsed.password : "",
    skip_verify: typeof parsed.skip_verify === "boolean" ? parsed.skip_verify : false,
  });
  const secretCandidates = [
    typeof parsed.token_secret === "string" ? parsed.token_secret : "",
    typeof parsed.password === "string" ? parsed.password : "",
  ];

  try {
    const response = await fetch(`${base.api}/connectors/portainer/test`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders
      },
      signal: AbortSignal.timeout(25_000),
      body: cleanBody
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        sanitizeTestRouteError(payload, "failed to test portainer connection", secretCandidates),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      sanitizeTestRouteError(error instanceof Error ? error.message : "", "failed to test portainer connection", secretCandidates),
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
