import { NextResponse } from "next/server";

import {
  backendAuthHeadersWithCookie,
  resolvedBackendBaseURLs,
  upstreamErrorPayload,
} from "../../../../../lib/backend";
import { markResponseNoStore } from "../../../../../lib/noStoreResponse";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

const UPSTREAM_TIMEOUT_MS = 15_000;

function desktopSessionJSON(payload: unknown, status: number): Response {
  return markResponseNoStore(NextResponse.json(payload, { status }));
}

export function validDesktopSessionID(value: string): boolean {
  const normalized = value.trim();
  return (
    normalized.length > 0 &&
    normalized.length <= 255 &&
    !/[\u0000-\u001f\u007f-\u009f/\\]/u.test(normalized)
  );
}

export async function DELETE(
  request: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  if (!isMutationRequestOriginAllowed(request)) {
    return desktopSessionJSON({ error: "forbidden origin" }, 403);
  }

  const { id } = await params;
  if (!validDesktopSessionID(id)) {
    return desktopSessionJSON({ error: "invalid desktop session id" }, 400);
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(
      `${base.api}/desktop/sessions/${encodeURIComponent(id.trim())}`,
      {
        method: "DELETE",
        cache: "no-store",
        signal: AbortSignal.timeout(UPSTREAM_TIMEOUT_MS),
        headers: backendAuthHeadersWithCookie(request),
      },
    );
    if (!response.ok) {
      const payload = await safeJSON(response);
      return desktopSessionJSON(
        upstreamErrorPayload(
          response.status,
          payload,
          "failed to close remote view session",
        ),
        response.status,
      );
    }

    return markResponseNoStore(new NextResponse(null, { status: 204 }));
  } catch {
    return desktopSessionJSON(
      { error: "remote view session endpoint unavailable" },
      502,
    );
  }
}

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
