import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../lib/proxyAuth";
import {
  notificationBackendUnavailableResponse,
  notificationJSONResponse,
  safeNotificationResponseJSON,
} from "../../../proxy";

export const dynamic = "force-dynamic";

export async function POST(request: Request, context: { params: Promise<{ channelId: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return notificationJSONResponse({ success: false, error: "forbidden origin" }, 403);
  }

  try {
    const { channelId } = await context.params;
    const base = await resolvedBackendBaseURLs();
    const authHeaders = backendAuthHeadersWithCookie(request);
    const response = await fetch(
      `${base.api}/notifications/channels/${encodeURIComponent(channelId)}/test`,
      {
        method: "POST",
        headers: authHeaders,
        cache: "no-store",
      },
    );
    const payload = await safeNotificationResponseJSON(response);
    if (!response.ok) {
      return notificationJSONResponse(
        payload ?? { success: false, error: "failed to send test notification" },
        response.status,
      );
    }
    if (payload === null) return notificationBackendUnavailableResponse(true);
    return notificationJSONResponse(payload);
  } catch {
    return notificationBackendUnavailableResponse(true);
  }
}
