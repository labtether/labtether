import { NextRequest, NextResponse } from "next/server";

import {
  backendAuthHeadersWithCookie,
  resolvedBackendBaseURLs,
  upstreamErrorPayload,
} from "../../../../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ assetId: string; action: string }>;
};

const allowedActions = new Set(["get", "set"]);

export async function POST(request: NextRequest, context: RouteContext) {
  const { assetId, action } = await context.params;
  const normalizedAction = action.trim().toLowerCase();

  if (!allowedActions.has(normalizedAction)) {
    return NextResponse.json({ error: "not found" }, { status: 404 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const url = `${base.api}/api/v1/nodes/${encodeURIComponent(assetId)}/clipboard/${encodeURIComponent(normalizedAction)}`;
    const headers = {
      ...backendAuthHeadersWithCookie(request),
      "Content-Type": request.headers.get("content-type") || "application/json",
    };
    const body = await request.text();

    const response = await fetch(url, {
      method: "POST",
      headers,
      body,
      cache: "no-store",
    });

    const payload = await response.json().catch(() => null);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(
          response.status,
          payload,
          `clipboard ${normalizedAction} failed (${response.status})`,
        ),
        { status: response.status || 502 },
      );
    }

    return NextResponse.json(payload ?? {}, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      {
        error:
          error instanceof Error
            ? error.message
            : `clipboard ${normalizedAction} endpoint unavailable`,
      },
      { status: 502 },
    );
  }
}
