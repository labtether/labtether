import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../../lib/backend";
import { readBoundedRequestBody, RequestBodyTooLargeError } from "../../../../../../../../lib/boundedBody";
import { isMutationRequestOriginAllowed } from "../../../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type Params = { params: Promise<{ assetId: string; snapshotId: string; action: string }> };

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}

const ALLOWED_ACTIONS = new Set(["rollback", "clone"]);

export async function POST(request: Request, { params }: Params) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { assetId, snapshotId, action } = await params;

  if (!ALLOWED_ACTIONS.has(action)) {
    return NextResponse.json({ error: "invalid action" }, { status: 400 });
  }

  const assetID = encodeURIComponent(assetId ?? "");
  const snapID = encodeURIComponent(snapshotId ?? "");
  const act = encodeURIComponent(action);
  let body: ArrayBuffer | undefined;
  if (action === "clone") {
    try {
      body = await readBoundedRequestBody(request, 1024 * 1024);
    } catch (error) {
      if (error instanceof RequestBodyTooLargeError) {
        return NextResponse.json({ error: "request body too large" }, { status: 413 });
      }
      return NextResponse.json({ error: "invalid request body" }, { status: 400 });
    }
  }
  try {
    const response = await fetch(
      `${base.api}/truenas/assets/${assetID}/snapshots/${snapID}/${act}`,
      {
        method: "POST",
        headers: body === undefined ? authHeaders : { ...authHeaders, "Content-Type": "application/json" },
        body,
      },
    );
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: `failed to ${action} snapshot` }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : `failed to ${action} snapshot` },
      { status: 502 },
    );
  }
}
