import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs, shouldUseSecureWebSocket } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

type StreamTicketRequest = {
  sessionId: string;
  terminalShell?: string;
};

type StreamTicketPayload = {
  session_id?: string;
  ticket?: string;
  expires_at?: string;
  stream_path?: string;
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
  const terminalShell = typeof body.terminalShell === "string" ? body.terminalShell.trim() : "";
  if (terminalShell.length > 160) {
    return NextResponse.json({ error: "terminalShell is too long" }, { status: 400 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/sessions/${encodeURIComponent(sessionId)}/stream-ticket`, {
      method: "POST",
      headers: {
        ...backendAuthHeadersWithCookie(request)
      }
    });

    const payload = (await safeJSON<StreamTicketPayload>(response)) ?? {};
    if (!response.ok || !payload.ticket) {
      return NextResponse.json(
        { error: payload.error || `failed to create stream ticket (${response.status})` },
        { status: response.status || 502 }
      );
    }

    const streamPath = payload.stream_path?.trim() || `/terminal/sessions/${encodeURIComponent(sessionId)}/stream?ticket=${encodeURIComponent(payload.ticket)}`;
    const streamPathWithShell = appendTerminalShell(streamPath, terminalShell);

    // Return streamPath for the client to build the WebSocket URL.
    // The client must construct the WS URL from its own window.location
    // because the server-side base.api is the Docker-internal hostname
    // (e.g. http://labtether:8080), which the browser can't reach.
    return NextResponse.json({
      sessionId,
      ticket: payload.ticket,
      expiresAt: payload.expires_at,
      streamPath: streamPathWithShell,
      secure: shouldUseSecureWebSocket(request),
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "stream ticket endpoint unavailable" },
      { status: 502 }
    );
  }
}

function appendTerminalShell(streamPath: string, terminalShell: string): string {
  const shell = terminalShell.trim();
  if (!shell) return streamPath;

  const [pathOnly, query = ""] = streamPath.split("?", 2);
  const params = new URLSearchParams(query);
  params.set("shell", shell);
  const encoded = params.toString();
  return encoded ? `${pathOnly}?${encoded}` : pathOnly;
}

async function safeJSON<T>(response: Response): Promise<T | null> {
  try {
    return (await response.json()) as T;
  } catch {
    return null;
  }
}
