import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";
import {
  safeSavedActionResponseJSON,
  savedActionBackendUnavailableResponse,
  savedActionJSONResponse,
  savedActionRequestErrorResponse,
  savedActionUpstreamErrorResponse,
  validateSavedActionEmptyMutationBody,
  validSavedActionID,
} from "../proxy";

export const dynamic = "force-dynamic";

type SavedActionRouteContext = { params: Promise<{ id: string }> };

export async function GET(request: Request, { params }: SavedActionRouteContext) {
  const { id } = await params;
  if (!validSavedActionID(id)) return savedActionJSONResponse({ error: "not found" }, 404);

  try {
    const base = await resolvedBackendBaseURLs();
    const authHeaders = backendAuthHeadersWithCookie(request);
    const response = await fetch(`${base.api}/api/v2/actions/${encodeURIComponent(id)}`, {
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(15_000),
    });
    const payload = await safeSavedActionResponseJSON(response);
    if (!response.ok) return savedActionUpstreamErrorResponse(response, payload, "failed to load saved action");
    return savedActionJSONResponse(payload ?? {}, response.status);
  } catch {
    return savedActionBackendUnavailableResponse();
  }
}

export async function DELETE(request: Request, { params }: SavedActionRouteContext) {
  if (!isMutationRequestOriginAllowed(request)) {
    return savedActionJSONResponse({ error: "forbidden origin" }, 403);
  }
  const { id } = await params;
  if (!validSavedActionID(id)) return savedActionJSONResponse({ error: "not found" }, 404);
  try {
    await validateSavedActionEmptyMutationBody(request);
  } catch (error) {
    return savedActionRequestErrorResponse(error) ?? savedActionJSONResponse({ error: "invalid request body" }, 400);
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const authHeaders = backendAuthHeadersWithCookie(request);
    const response = await fetch(`${base.api}/api/v2/actions/${encodeURIComponent(id)}`, {
      method: "DELETE",
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(15_000),
    });
    const payload = await safeSavedActionResponseJSON(response);
    if (!response.ok) return savedActionUpstreamErrorResponse(response, payload, "failed to delete saved action");
    return savedActionJSONResponse(payload ?? {}, response.status);
  } catch {
    return savedActionBackendUnavailableResponse();
  }
}
