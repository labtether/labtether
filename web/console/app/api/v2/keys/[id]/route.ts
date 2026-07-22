import { NextRequest } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";
import {
  apiKeyBackendUnavailableResponse,
  apiKeyJSONResponse,
  apiKeyRequestErrorResponse,
  apiKeyUpstreamErrorResponse,
  readAPIKeyRequestJSON,
  safeAPIKeyResponseJSON,
  validAPIKeyID,
} from "../proxy";

export const dynamic = "force-dynamic";

export async function PATCH(request: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return apiKeyJSONResponse({ error: "forbidden origin" }, 403);
  }

  const { id } = await params;
  if (!validAPIKeyID(id)) return apiKeyJSONResponse({ error: "not found" }, 404);
  let body: Record<string, unknown>;
  try {
    body = await readAPIKeyRequestJSON(request);
  } catch (error) {
    return apiKeyRequestErrorResponse(error) ?? apiKeyJSONResponse({ error: "invalid request body" }, 400);
  }
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(`${base.api}/api/v2/keys/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    const payload = await safeAPIKeyResponseJSON(response);
    if (!response.ok) return apiKeyUpstreamErrorResponse(response, payload, "failed to update API key");
    return apiKeyJSONResponse(payload ?? {}, response.status);
  } catch {
    return apiKeyBackendUnavailableResponse();
  }
}

export async function DELETE(request: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return apiKeyJSONResponse({ error: "forbidden origin" }, 403);
  }

  const { id } = await params;
  if (!validAPIKeyID(id)) return apiKeyJSONResponse({ error: "not found" }, 404);
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(`${base.api}/api/v2/keys/${encodeURIComponent(id)}`, {
      method: "DELETE",
      headers: authHeaders,
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    const payload = await safeAPIKeyResponseJSON(response);
    if (!response.ok) return apiKeyUpstreamErrorResponse(response, payload, "failed to revoke API key");
    return apiKeyJSONResponse(payload ?? {}, response.status);
  } catch {
    return apiKeyBackendUnavailableResponse();
  }
}
