import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

const POWER_ACTIONS = new Set(["reboot", "shutdown"]);

/** Restart or shut down a device through its agent: POST { action: "reboot" | "shutdown" }. */
export async function POST(request: Request, { params }: { params: Promise<{ id: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }
  const { id } = await params;
  const body = (await request.json().catch(() => ({}))) as { action?: unknown };
  const action = typeof body.action === "string" ? body.action : "";
  if (!POWER_ACTIONS.has(action)) {
    return NextResponse.json({ error: "action must be reboot or shutdown" }, { status: 400 });
  }
  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/api/v2/assets/${encodeURIComponent(id)}/${action}`, {
      method: "POST",
      cache: "no-store",
      headers: backendAuthHeadersWithCookie(request),
      signal: AbortSignal.timeout(60_000),
    });
    const payload = await response.json().catch(() => null);
    return NextResponse.json(payload ?? {}, { status: response.status });
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "power action failed" }, { status: 502 });
  }
}
