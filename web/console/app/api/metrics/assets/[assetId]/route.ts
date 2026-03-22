import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ assetId: string }>;
};

export async function GET(request: Request, context: RouteContext) {
  const { assetId } = await context.params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  const url = new URL(request.url);
  const backendURL = new URL(`${base.api}/metrics/assets/${encodeURIComponent(assetId)}`);

  const window = url.searchParams.get("window");
  if (window) {
    backendURL.searchParams.set("window", window);
  }
  const step = url.searchParams.get("step");
  if (step) {
    backendURL.searchParams.set("step", step);
  }

  try {
    const response = await fetch(backendURL.toString(), {
      cache: "no-store",
      headers: authHeaders
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        payload ?? { error: "failed to load asset metrics" },
        { status: response.status }
      );
    }

    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to query backend telemetry" },
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
