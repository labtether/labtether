import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs, upstreamErrorPayload } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

export async function DELETE(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  try {
    const body = await request.json();
    const response = await fetch(`${base.api}/auth/2fa`, {
      method: "DELETE",
      headers: { "Content-Type": "application/json", ...backendAuthHeadersWithCookie(request) },
      body: JSON.stringify(body),
      cache: "no-store",
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to disable 2FA"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "2FA disable endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}
