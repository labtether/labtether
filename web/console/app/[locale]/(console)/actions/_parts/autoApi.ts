import { safeJSON } from "../../../../lib/api";
import { ensureRecord } from "../../../../lib/responseGuards";
import { sanitizeErrorMessage } from "../../../../lib/sanitizeErrorMessage";

/**
 * The hub's own words for a failed request. Hub v2 errors carry a code in `error` ("validation") and the sentence in
 * `message` ("name is required"), so the sentence wins; status codes and server internals become the fallback.
 */
export function hubError(payload: unknown, fallback: string): string {
  const record = ensureRecord(payload);
  const message = typeof record?.message === "string" ? record.message : "";
  const code = typeof record?.error === "string" ? record.error : "";
  const text = message || (code.includes(" ") ? code : "");
  return sanitizeErrorMessage(text, fallback);
}

/** A write to the hub (POST, PATCH, DELETE). Throws an Error with the hub's sentence when it refuses. */
export async function autoMutate<T = unknown>(url: string, method: "POST" | "PATCH" | "DELETE", fallback: string, body?: unknown): Promise<T> {
  const response = await fetch(url, {
    method,
    cache: "no-store",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const payload = await safeJSON(response);
  if (!response.ok) throw new Error(hubError(payload, fallback));
  return payload as T;
}

/** The message of a thrown error, or the fallback. */
export function errorText(cause: unknown, fallback: string): string {
  return sanitizeErrorMessage(cause instanceof Error ? cause.message : "", fallback);
}
