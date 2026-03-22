import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs, shouldUseSecureWebSocket } from "../../../../lib/backend";

type SpiceTicketRequest = {
  sessionId: string;
};

type SpiceTicketPayload = {
  session_id?: string;
  ticket?: string;
  expires_at?: string;
  stream_path?: string;
  password?: string;
  type?: string;
  ca?: string;
  proxy?: string;
  error?: string;
};

export async function POST(request: Request) {
  let body: SpiceTicketRequest;
  try {
    body = (await request.json()) as SpiceTicketRequest;
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }

  const sessionId = body.sessionId?.trim();
  if (!sessionId) {
    return NextResponse.json({ error: "sessionId is required" }, { status: 400 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/desktop/sessions/${encodeURIComponent(sessionId)}/spice-ticket`, {
      method: "POST",
      cache: "no-store",
      headers: {
        ...backendAuthHeadersWithCookie(request),
      },
    });
    const payload = (await safeJSON<SpiceTicketPayload>(response)) ?? {};
    if (!response.ok) {
      return NextResponse.json({ error: payload.error || "failed to fetch spice ticket" }, { status: response.status || 502 });
    }

    // Modern same-origin transport shape.
    if (payload.stream_path && payload.password) {
      return NextResponse.json({
        sessionId,
        ticket: payload.ticket,
        expiresAt: payload.expires_at,
        streamPath: payload.stream_path,
        secure: shouldUseSecureWebSocket(request),
        password: payload.password,
        type: payload.type,
        ca: payload.ca,
        proxy: payload.proxy,
      });
    }

    return NextResponse.json({ error: "spice stream path unavailable from backend" }, { status: 502 });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to fetch spice ticket" },
      { status: 502 },
    );
  }
}

async function safeJSON<T>(response: Response): Promise<T | null> {
  try {
    return (await response.json()) as T;
  } catch {
    return null;
  }
}
