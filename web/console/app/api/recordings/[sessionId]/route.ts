import {
  backendAuthHeadersWithCookie,
  resolvedBackendBaseURLs,
} from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";
import {
  desktopProxyJSON,
  desktopUpstreamError,
  desktopUpstreamTimeoutMs,
  safeDesktopResponseJSON,
  validDesktopResourceID,
} from "../../desktop/proxy";

export const dynamic = "force-dynamic";

export async function POST(
  request: Request,
  { params }: { params: Promise<{ sessionId: string }> },
) {
  if (!isMutationRequestOriginAllowed(request)) {
    return desktopProxyJSON({ error: "forbidden origin" }, 403);
  }

  const { sessionId } = await params;
  if (!validDesktopResourceID(sessionId)) {
    return desktopProxyJSON({ error: "invalid recording session id" }, 400);
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(
      `${base.api}/recordings/${encodeURIComponent(sessionId.trim())}`,
      {
        method: "POST",
        cache: "no-store",
        signal: AbortSignal.timeout(desktopUpstreamTimeoutMs),
        headers: backendAuthHeadersWithCookie(request),
      },
    );
    const payload = await safeDesktopResponseJSON(response);
    if (!response.ok) {
      return desktopUpstreamError(
        response,
        payload,
        "failed to stop recording",
      );
    }
    return desktopProxyJSON(payload ?? { stopped: true }, response.status);
  } catch {
    return desktopProxyJSON({ error: "failed to stop recording" }, 502);
  }
}
