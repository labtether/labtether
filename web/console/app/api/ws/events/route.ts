import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs, shouldUseSecureWebSocket } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const secure = shouldUseSecureWebSocket(request);
    const streamPath = "/ws/events";

    const authHeaders = backendAuthHeadersWithCookie(request);

    // Request a one-time ticket from the backend (avoids exposing bearer token in WS URL).
    const ticketRes = await fetch(`${base.api}/ws/events/ticket`, {
      method: "POST",
      headers: authHeaders,
      cache: "no-store",
    });

    if (ticketRes.ok) {
      const ticketData = (await ticketRes.json()) as { ticket?: string };
      if (ticketData.ticket) {
        return NextResponse.json({
          streamPath: `${streamPath}?ticket=${encodeURIComponent(ticketData.ticket)}`,
          secure,
        });
      }
    }

    // Fallback: return stream path without ticket (same-origin cookie auth only).
    return NextResponse.json({ streamPath, secure });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "ws endpoint unavailable" },
      { status: 502 }
    );
  }
}
