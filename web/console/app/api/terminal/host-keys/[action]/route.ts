import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

type RouteParams = { params: Promise<{ action: string }> };

const ACTIONS = new Set(["check", "trust", "forget"]);

/** SSH host keys for Quick connect and bookmarks: check what a host shows, trust it, or remove a saved key. */
export async function POST(request: Request, props: RouteParams) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }
  const { action } = await props.params;
  if (!ACTIONS.has(action)) {
    return NextResponse.json({ error: "not found" }, { status: 404 });
  }
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }
  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/host-keys/${action}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...backendAuthHeadersWithCookie(request) },
      body: JSON.stringify(body),
    });
    let payload: unknown = null;
    try {
      payload = await response.json();
    } catch {
      payload = null;
    }
    return NextResponse.json(payload ?? { error: "host key request failed" }, { status: response.status || 502 });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "host key endpoint unavailable" },
      { status: 502 },
    );
  }
}
