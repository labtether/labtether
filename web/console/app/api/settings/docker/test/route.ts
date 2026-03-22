import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { sanitizeTestRouteError } from "../../testErrorSanitizer";

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/connectors/docker/test`, {
      method: "POST",
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(20_000),
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        sanitizeTestRouteError(payload, "failed to test docker connection"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      sanitizeTestRouteError(error instanceof Error ? error.message : "", "failed to test docker connection"),
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
