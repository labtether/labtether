import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs, upstreamErrorPayload } from "../../../../../lib/backend";
import { readBoundedRequestBody, RequestBodyTooLargeError } from "../../../../../lib/boundedBody";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

const maxPasswordBodyBytes = 8 * 1024;

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  let parsed: unknown;
  try {
    const rawBody = await readBoundedRequestBody(request, maxPasswordBodyBytes);
    parsed = JSON.parse(new TextDecoder().decode(rawBody));
  } catch (error) {
    if (error instanceof RequestBodyTooLargeError) {
      return NextResponse.json({ error: "request body too large" }, { status: 413 });
    }
    return NextResponse.json({ error: "invalid request body" }, { status: 400 });
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    return NextResponse.json({ error: "invalid request body" }, { status: 400 });
  }
  const values = parsed as Record<string, unknown>;
  const currentPassword = typeof values.current_password === "string" ? values.current_password : "";
  const newPassword = typeof values.new_password === "string" ? values.new_password : "";
  if (currentPassword.length > 1024 || newPassword.length > 1024) {
    return NextResponse.json({ error: "password value is too long" }, { status: 400 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/auth/me/password`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request),
      },
      body: JSON.stringify({
        current_password: currentPassword,
        new_password: newPassword,
      }),
      signal: AbortSignal.timeout(15_000),
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to change password"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {}, { status: response.status });
  } catch {
    return NextResponse.json({ error: "password change endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
