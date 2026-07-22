import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";
import {
  readSavedActionRequestJSON,
  safeSavedActionResponseJSON,
  savedActionBackendUnavailableResponse,
  savedActionJSONResponse,
  savedActionRequestErrorResponse,
  savedActionUpstreamErrorResponse,
} from "./proxy";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const query = new URL(request.url).search;
  try {
    const base = await resolvedBackendBaseURLs();
    const authHeaders = backendAuthHeadersWithCookie(request);
    const response = await fetch(`${base.api}/api/v2/actions${query}`, {
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(15_000),
    });
    const payload = await safeSavedActionResponseJSON(response);
    if (!response.ok) return savedActionUpstreamErrorResponse(response, payload, "failed to load saved actions");
    return savedActionJSONResponse(payload ?? { data: [], meta: { total: 0 } }, response.status);
  } catch {
    return savedActionBackendUnavailableResponse();
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return savedActionJSONResponse({ error: "forbidden origin" }, 403);
  }

  let body: Record<string, unknown>;
  try {
    body = await readSavedActionRequestJSON(request);
  } catch (error) {
    return savedActionRequestErrorResponse(error) ?? savedActionJSONResponse({ error: "invalid request body" }, 400);
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const authHeaders = backendAuthHeadersWithCookie(request);
    const response = await fetch(`${base.api}/api/v2/actions`, {
      method: "POST",
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    const payload = await safeSavedActionResponseJSON(response);
    if (!response.ok) return savedActionUpstreamErrorResponse(response, payload, "failed to create saved action");
    return savedActionJSONResponse(payload ?? {}, response.status);
  } catch {
    return savedActionBackendUnavailableResponse();
  }
}
