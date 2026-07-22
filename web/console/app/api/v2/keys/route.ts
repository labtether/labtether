import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";
import {
  apiKeyBackendUnavailableResponse,
  apiKeyJSONResponse,
  apiKeyRequestErrorResponse,
  apiKeyUpstreamErrorResponse,
  readAPIKeyRequestJSON,
  safeAPIKeyResponseJSON,
} from "./proxy";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(`${base.api}/api/v2/keys`, {
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(15_000),
    });
    const payload = await safeAPIKeyResponseJSON(response);
    if (!response.ok) return apiKeyUpstreamErrorResponse(response, payload, "failed to load API keys");
    return apiKeyJSONResponse(payload ?? { data: [] }, response.status);
  } catch {
    return apiKeyBackendUnavailableResponse();
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return apiKeyJSONResponse({ error: "forbidden origin" }, 403);
  }

  let body: Record<string, unknown>;
  try {
    body = await readAPIKeyRequestJSON(request);
  } catch (error) {
    return apiKeyRequestErrorResponse(error) ?? apiKeyJSONResponse({ error: "invalid request body" }, 400);
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(`${base.api}/api/v2/keys`, {
      method: "POST",
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    const payload = await safeAPIKeyResponseJSON(response);
    if (!response.ok) return apiKeyUpstreamErrorResponse(response, payload, "failed to create API key");
    return apiKeyJSONResponse(payload ?? {}, response.status);
  } catch {
    return apiKeyBackendUnavailableResponse();
  }
}
