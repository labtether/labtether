import {
  backendAuthHeadersWithCookie,
  resolvedBackendBaseURLs,
} from "../../../../../lib/backend";
import { markResponseNoStore } from "../../../../../lib/noStoreResponse";
import {
  desktopProxyJSON,
  desktopUpstreamError,
  safeDesktopResponseJSON,
  validDesktopResourceID,
} from "../../../desktop/proxy";

export const dynamic = "force-dynamic";

// Streams GET /recordings/{recordingID}/download from the Hub. The folder
// shares the [sessionId] segment name with the stop route next to it, but
// here the value is the recording ID from the recordings list. The Hub
// enforces operator+ access, visibility (404) and in-progress (409) rules;
// this proxy only validates the ID and passes the attachment through.
// No total timeout is applied so long recordings are not cut off mid-stream.
export async function GET(
  request: Request,
  { params }: { params: Promise<{ sessionId: string }> },
) {
  const { sessionId: recordingID } = await params;
  if (!validDesktopResourceID(recordingID)) {
    return desktopProxyJSON({ error: "invalid recording id" }, 400);
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(
      `${base.api}/recordings/${encodeURIComponent(recordingID.trim())}/download`,
      {
        cache: "no-store",
        headers: backendAuthHeadersWithCookie(request),
      },
    );
    if (!response.ok || !response.body) {
      const payload = await safeDesktopResponseJSON(response);
      return desktopUpstreamError(response, payload, "failed to download recording");
    }

    const headers = new Headers({
      "Content-Type": "application/octet-stream",
      "X-Content-Type-Options": "nosniff",
    });
    const disposition = response.headers.get("content-disposition");
    headers.set("Content-Disposition", disposition || "attachment");
    const length = response.headers.get("content-length");
    if (length) {
      headers.set("Content-Length", length);
    }
    return markResponseNoStore(new Response(response.body, { status: 200, headers }));
  } catch {
    return desktopProxyJSON({ error: "failed to download recording" }, 502);
  }
}
