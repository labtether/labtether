import { NextRequest, NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs, upstreamErrorPayload } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type SyncRequestBody = {
  host_asset_id?: string;
  host?: string;
};

export async function POST(request: NextRequest) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  let body: SyncRequestBody | null = null;
  try {
    body = (await request.json()) as SyncRequestBody;
  } catch {
    body = null;
  }

  try {
    const hostFromQuery = request.nextUrl.searchParams.get("host");
    const targetHostRaw = hostFromQuery ?? body?.host_asset_id ?? body?.host ?? "";
    const targetHost = normalizeSelectorValue(targetHostRaw);
    if (targetHost === null) {
      return NextResponse.json({ error: "invalid host selector" }, { status: 400 });
    }
    const qs = targetHost ? `?host=${encodeURIComponent(targetHost)}` : "";

    const response = await fetch(`${base.api}/api/v1/services/web/sync${qs}`, {
      method: "POST",
      cache: "no-store",
      headers: {
        ...authHeaders,
        "content-type": "application/json",
      },
      body: "{}",
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to trigger web service sync"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "service sync endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

function normalizeSelectorValue(raw: string): string | null {
  const value = raw.trim();
  if (value === "") {
    return "";
  }
  if (value.length > 128) {
    return null;
  }
  if (!/^[a-zA-Z0-9][a-zA-Z0-9._:-]*$/.test(value)) {
    return null;
  }
  return value;
}
