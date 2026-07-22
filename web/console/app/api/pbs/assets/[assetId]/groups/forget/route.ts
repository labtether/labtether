import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type Params = { params: Promise<{ assetId: string }> };

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}

async function proxyForget(request: Request, { params }: Params) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId } = await params;
  const assetID = encodeURIComponent(assetId ?? "");

  const incomingURL = new URL(request.url);
  const upstreamURL = new URL(`${base.api}/pbs/assets/${assetID}/groups/forget`);
  for (const key of ["store", "backup-type", "backup-id"]) {
    const value = incomingURL.searchParams.get(key)?.trim();
    if (value) upstreamURL.searchParams.set(key, value);
  }

  // Translate the previous console's POST body so an already-open tab remains
  // compatible across a Hub update.
  if (![...upstreamURL.searchParams].length && request.method === "POST") {
    let body: Record<string, unknown> = {};
    try {
      const parsed = await request.json();
      if (parsed && typeof parsed === "object") body = parsed as Record<string, unknown>;
    } catch { /* backend returns the validation error */ }
    const legacyFields: Array<[string, unknown]> = [
      ["store", body.store],
      ["backup-type", body.backup_type ?? body["backup-type"]],
      ["backup-id", body.backup_id ?? body["backup-id"]],
    ];
    for (const [key, value] of legacyFields) {
      if (value !== undefined && value !== null && String(value).trim()) {
        upstreamURL.searchParams.set(key, String(value).trim());
      }
    }
  }

  try {
    const response = await fetch(upstreamURL, {
      method: "DELETE",
      headers: authHeaders,
      signal: AbortSignal.timeout(30_000),
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to forget group" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "failed to forget group" }, { status: 502 });
  }
}

export const DELETE = proxyForget;
export const POST = proxyForget;
