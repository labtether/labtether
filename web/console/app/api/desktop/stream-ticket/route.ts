import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs, shouldUseSecureWebSocket } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

type StreamTicketRequest = {
  sessionId: string;
};

type StreamTicketPayload = {
  session_id?: string;
  ticket?: string;
  expires_at?: string;
  stream_path?: string;
  vnc_password?: string;
  error?: string;
};

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  let body: StreamTicketRequest;
  try {
    body = (await request.json()) as StreamTicketRequest;
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }

  const sessionId = body.sessionId?.trim();
  if (!sessionId) {
    return NextResponse.json({ error: "sessionId is required" }, { status: 400 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/desktop/sessions/${encodeURIComponent(sessionId)}/stream-ticket`, {
      method: "POST",
      headers: {
        ...backendAuthHeadersWithCookie(request)
      }
    });

    const payload = (await safeJSON<StreamTicketPayload>(response)) ?? {};
    if (!response.ok || !payload.ticket) {
      return NextResponse.json(
        { error: payload.error || `failed to create remote view stream ticket (${response.status})` },
        { status: response.status || 502 }
      );
    }

    const streamPath = payload.stream_path?.trim() || `/desktop/sessions/${encodeURIComponent(sessionId)}/stream?ticket=${encodeURIComponent(payload.ticket)}`;

    // Return streamPath for the client to build the WebSocket URL.
    // The client must construct the WS URL from its own window.location
    // because the server-side base.api is the Docker-internal hostname
    // (e.g. http://labtether:8080), which the browser can't reach.
    return NextResponse.json({
      sessionId,
      ticket: payload.ticket,
      expiresAt: payload.expires_at,
      streamPath,
      vncPassword: payload.vnc_password,
      secure: shouldUseSecureWebSocket(request),
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "remote view stream ticket endpoint unavailable" },
      { status: 502 }
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
