import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";
import {
  notificationBackendUnavailableResponse,
  notificationJSONResponse,
  notificationRequestErrorResponse,
  readNotificationRequestJSON,
  safeNotificationResponseJSON,
} from "../proxy";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const authHeaders = backendAuthHeadersWithCookie(request);
    const incoming = new URL(request.url);
    const backendURL = new URL(`${base.api}/notifications/channels`);
    for (const [key, value] of incoming.searchParams.entries()) {
      backendURL.searchParams.set(key, value);
    }
    const response = await fetch(backendURL.toString(), { cache: "no-store", headers: authHeaders });
    const payload = await safeNotificationResponseJSON(response);
    if (!response.ok) {
      return notificationJSONResponse(payload ?? { error: "failed to load notification channels" }, response.status);
    }
    if (payload === null) return notificationBackendUnavailableResponse();
    return notificationJSONResponse(payload);
  } catch {
    return notificationBackendUnavailableResponse();
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return notificationJSONResponse({ error: "forbidden origin" }, 403);
  }

  let body: Record<string, unknown>;
  try {
    body = await readNotificationRequestJSON(request);
  } catch (error) {
    return notificationRequestErrorResponse(error) ?? notificationJSONResponse({ error: "invalid request body" }, 400);
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const authHeaders = backendAuthHeadersWithCookie(request);
    const response = await fetch(`${base.api}/notifications/channels`, {
      method: "POST",
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
    const payload = await safeNotificationResponseJSON(response);
    if (!response.ok) {
      return notificationJSONResponse(payload ?? { error: "failed to create notification channel" }, response.status);
    }
    if (payload === null) return notificationBackendUnavailableResponse();
    return notificationJSONResponse(payload);
  } catch {
    return notificationBackendUnavailableResponse();
  }
}
