import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import {
  notificationBackendUnavailableResponse,
  notificationJSONResponse,
  safeNotificationResponseJSON,
} from "../proxy";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const authHeaders = backendAuthHeadersWithCookie(request);
    const incoming = new URL(request.url);
    const backendURL = new URL(`${base.api}/notifications/history`);
    for (const [key, value] of incoming.searchParams.entries()) {
      backendURL.searchParams.set(key, value);
    }
    const response = await fetch(backendURL.toString(), { cache: "no-store", headers: authHeaders });
    const payload = await safeNotificationResponseJSON(response);
    if (!response.ok) {
      return notificationJSONResponse(payload ?? { error: "failed to load notification history" }, response.status);
    }
    if (payload === null) return notificationBackendUnavailableResponse();
    return notificationJSONResponse(payload);
  } catch {
    return notificationBackendUnavailableResponse();
  }
}
