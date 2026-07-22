import {
  backendAuthHeadersWithCookie,
  resolvedBackendBaseURLs,
} from "../../../../lib/backend";
import {
  desktopProxyJSON,
  desktopUpstreamError,
  desktopUpstreamTimeoutMs,
  safeDesktopResponseJSON,
  validDesktopResourceID,
} from "../../desktop/proxy";

export const dynamic = "force-dynamic";

export async function GET(
  request: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  if (!validDesktopResourceID(id)) {
    return desktopProxyJSON({ error: "invalid asset id" }, 400);
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(
      `${base.api}/assets/${encodeURIComponent(id.trim())}/displays`,
      {
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
        "failed to query displays",
      );
    }
    return desktopProxyJSON(payload ?? { displays: [] }, response.status);
  } catch {
    return desktopProxyJSON({ error: "display query failed" }, 502);
  }
}
