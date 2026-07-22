import {
  backendAuthHeadersWithCookie,
  resolvedBackendBaseURLs,
} from "../../../lib/backend";
import {
  readBoundedRequestBody,
  RequestBodyTooLargeError,
} from "../../../lib/boundedBody";
import { isMutationRequestOriginAllowed } from "../../../lib/proxyAuth";
import {
  desktopProxyJSON,
  desktopUpstreamError,
  desktopUpstreamTimeoutMs,
  safeDesktopResponseJSON,
  validDesktopResourceID,
} from "../desktop/proxy";

export const dynamic = "force-dynamic";

const MAX_RECORDING_REQUEST_BODY_BYTES = 4096;

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/recordings`, {
      cache: "no-store",
      signal: AbortSignal.timeout(desktopUpstreamTimeoutMs),
      headers: backendAuthHeadersWithCookie(request),
    });
    const payload = await safeDesktopResponseJSON(response);
    if (!response.ok) {
      return desktopUpstreamError(
        response,
        payload,
        "failed to query recordings",
      );
    }
    return desktopProxyJSON(payload ?? { recordings: [] }, response.status);
  } catch {
    return desktopProxyJSON({ error: "recordings query failed" }, 502);
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return desktopProxyJSON({ error: "forbidden origin" }, 403);
  }

  let raw: ArrayBuffer;
  try {
    raw = await readBoundedRequestBody(
      request,
      MAX_RECORDING_REQUEST_BODY_BYTES,
    );
  } catch (error) {
    if (error instanceof RequestBodyTooLargeError) {
      return desktopProxyJSON({ error: "request body too large" }, 413);
    }
    return desktopProxyJSON({ error: "invalid recording request" }, 400);
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(raw));
  } catch {
    return desktopProxyJSON({ error: "invalid recording request" }, 400);
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    return desktopProxyJSON({ error: "invalid recording request" }, 400);
  }
  const sessionID =
    typeof (parsed as Record<string, unknown>).session_id === "string"
      ? (parsed as Record<string, string>).session_id.trim()
      : "";
  if (!validDesktopResourceID(sessionID)) {
    return desktopProxyJSON({ error: "invalid recording session id" }, 400);
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/recordings`, {
      method: "POST",
      cache: "no-store",
      signal: AbortSignal.timeout(desktopUpstreamTimeoutMs),
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request),
      },
      body: JSON.stringify({ session_id: sessionID }),
    });
    const payload = await safeDesktopResponseJSON(response);
    if (!response.ok) {
      return desktopUpstreamError(
        response,
        payload,
        "failed to start recording",
      );
    }
    return desktopProxyJSON(
      payload ?? { error: "failed to start recording" },
      response.status,
    );
  } catch {
    return desktopProxyJSON({ error: "failed to start recording" }, 502);
  }
}
