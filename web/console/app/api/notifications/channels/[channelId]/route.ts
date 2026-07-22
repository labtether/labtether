import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";
import {
  notificationBackendUnavailableResponse,
  notificationJSONResponse,
  notificationRequestErrorResponse,
  readNotificationRequestJSON,
  safeNotificationResponseJSON,
} from "../../proxy";

export const dynamic = "force-dynamic";

function backendPath(baseURL: string, channelId: string): string {
  return `${baseURL}/notifications/channels/${encodeURIComponent(channelId)}`;
}

export async function GET(request: Request, context: { params: Promise<{ channelId: string }> }) {
  try {
    const { channelId } = await context.params;
    const base = await resolvedBackendBaseURLs();
    const authHeaders = backendAuthHeadersWithCookie(request);
    const response = await fetch(backendPath(base.api, channelId), { cache: "no-store", headers: authHeaders });
    const payload = await safeNotificationResponseJSON(response);
    if (!response.ok) {
      return notificationJSONResponse(payload ?? { error: "failed to load notification channel" }, response.status);
    }
    if (payload === null) return notificationBackendUnavailableResponse();
    return notificationJSONResponse(payload);
  } catch {
    return notificationBackendUnavailableResponse();
  }
}

export async function PATCH(request: Request, context: { params: Promise<{ channelId: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return notificationJSONResponse({ error: "forbidden origin" }, 403);
  }

  return mutateChannel(request, context, "PATCH");
}

export async function PUT(request: Request, context: { params: Promise<{ channelId: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return notificationJSONResponse({ error: "forbidden origin" }, 403);
  }

  return mutateChannel(request, context, "PUT");
}

export async function DELETE(request: Request, context: { params: Promise<{ channelId: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return notificationJSONResponse({ error: "forbidden origin" }, 403);
  }

  try {
    const { channelId } = await context.params;
    const base = await resolvedBackendBaseURLs();
    const authHeaders = backendAuthHeadersWithCookie(request);
    const response = await fetch(backendPath(base.api, channelId), {
      method: "DELETE",
      headers: authHeaders,
      cache: "no-store",
    });
    const payload = await safeNotificationResponseJSON(response);
    if (!response.ok) {
      return notificationJSONResponse(payload ?? { error: "failed to delete notification channel" }, response.status);
    }
    if (payload === null) return notificationBackendUnavailableResponse();
    return notificationJSONResponse(payload);
  } catch {
    return notificationBackendUnavailableResponse();
  }
}

async function mutateChannel(
  request: Request,
  context: { params: Promise<{ channelId: string }> },
  method: "PATCH" | "PUT",
) {
  let body: Record<string, unknown>;
  try {
    body = await readNotificationRequestJSON(request);
  } catch (error) {
    return notificationRequestErrorResponse(error) ?? notificationJSONResponse({ error: "invalid request body" }, 400);
  }

  try {
    const { channelId } = await context.params;
    const base = await resolvedBackendBaseURLs();
    const authHeaders = backendAuthHeadersWithCookie(request);
    const response = await fetch(backendPath(base.api, channelId), {
      method,
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
    const payload = await safeNotificationResponseJSON(response);
    if (!response.ok) {
      return notificationJSONResponse(payload ?? { error: "failed to update notification channel" }, response.status);
    }
    if (payload === null) return notificationBackendUnavailableResponse();
    return notificationJSONResponse(payload);
  } catch {
    return notificationBackendUnavailableResponse();
  }
}
