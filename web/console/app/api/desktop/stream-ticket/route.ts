import {
  backendAuthHeadersWithCookie,
  resolvedBackendBaseURLs,
  shouldUseSecureWebSocket,
} from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";
import {
  desktopProxyJSON,
  desktopUpstreamError,
  desktopUpstreamTimeoutMs,
  readDesktopSessionIDRequest,
  safeDesktopResponseJSON,
} from "../proxy";

export const dynamic = "force-dynamic";

type StreamTicketPayload = {
  ticket?: string;
  expires_at?: string;
  stream_path?: string;
  vnc_password?: string;
};

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return desktopProxyJSON({ error: "forbidden origin" }, 403);
  }

  const parsed = await readDesktopSessionIDRequest(request);
  if ("error" in parsed) {
    return desktopProxyJSON({ error: parsed.error }, parsed.status);
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(
      `${base.api}/desktop/sessions/${encodeURIComponent(parsed.sessionID)}/stream-ticket`,
      {
        method: "POST",
        cache: "no-store",
        signal: AbortSignal.timeout(desktopUpstreamTimeoutMs),
        headers: backendAuthHeadersWithCookie(request),
      },
    );

    const payload = ((await safeDesktopResponseJSON(response)) ??
      {}) as StreamTicketPayload;
    if (!response.ok || typeof payload.ticket !== "string" || !payload.ticket) {
      return desktopUpstreamError(
        response,
        payload,
        "failed to create remote view stream ticket",
      );
    }

    const streamPath =
      typeof payload.stream_path === "string" && payload.stream_path.trim()
        ? payload.stream_path.trim()
        : `/desktop/sessions/${encodeURIComponent(parsed.sessionID)}/stream?ticket=${encodeURIComponent(payload.ticket)}`;
    if (
      streamPath.length > 4096 ||
      /[\u0000-\u001f\u007f-\u009f]/u.test(streamPath)
    ) {
      return desktopProxyJSON({ error: "invalid stream ticket response" }, 502);
    }

    return desktopProxyJSON({
      sessionId: parsed.sessionID,
      ticket: payload.ticket,
      expiresAt: payload.expires_at,
      streamPath,
      vncPassword: payload.vnc_password,
      secure: shouldUseSecureWebSocket(request),
    });
  } catch {
    return desktopProxyJSON(
      { error: "remote view stream ticket endpoint unavailable" },
      502,
    );
  }
}
