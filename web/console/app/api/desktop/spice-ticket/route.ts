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

type SpiceTicketPayload = {
  ticket?: string;
  expires_at?: string;
  stream_path?: string;
  password?: string;
  type?: string;
  ca?: string;
  proxy?: string;
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
      `${base.api}/desktop/sessions/${encodeURIComponent(parsed.sessionID)}/spice-ticket`,
      {
        method: "POST",
        cache: "no-store",
        signal: AbortSignal.timeout(desktopUpstreamTimeoutMs),
        headers: backendAuthHeadersWithCookie(request),
      },
    );
    const payload = ((await safeDesktopResponseJSON(response)) ??
      {}) as SpiceTicketPayload;
    if (!response.ok) {
      return desktopUpstreamError(
        response,
        payload,
        "failed to fetch SPICE ticket",
      );
    }

    const streamPath =
      typeof payload.stream_path === "string" ? payload.stream_path.trim() : "";
    if (
      !streamPath ||
      streamPath.length > 4096 ||
      /[\u0000-\u001f\u007f-\u009f]/u.test(streamPath) ||
      typeof payload.password !== "string"
    ) {
      return desktopProxyJSON(
        { error: "SPICE stream path unavailable from backend" },
        502,
      );
    }

    return desktopProxyJSON({
      sessionId: parsed.sessionID,
      ticket: payload.ticket,
      expiresAt: payload.expires_at,
      streamPath,
      secure: shouldUseSecureWebSocket(request),
      password: payload.password,
      type: payload.type,
      ca: payload.ca,
      proxy: payload.proxy,
    });
  } catch {
    return desktopProxyJSON(
      { error: "SPICE ticket endpoint unavailable" },
      502,
    );
  }
}
