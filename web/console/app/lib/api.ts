import { ensureRecord } from "./responseGuards";

/**
 * Safe JSON parsing — returns null on failure instead of throwing.
 */
export async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

/**
 * Extract error message from an API error response payload.
 * Looks for .error field (string), falls back to the provided default.
 */
export function extractError(payload: unknown, fallback: string): string {
  const record = ensureRecord(payload);
  if (record !== null) {
    const message = record["error"];
    if (typeof message === "string" && message.trim() !== "") {
      return message;
    }
  }
  return fallback;
}

/**
 * Client-side API fetch with standard defaults:
 * - cache: "no-store"
 * - Automatically adds Content-Type for JSON bodies
 * - Returns { response, data } where data is parsed JSON or null
 */
export async function apiFetch<T = unknown>(
  url: string,
  options?: RequestInit & { json?: unknown },
): Promise<{ response: Response; data: T | null }> {
  const { json, ...rest } = options ?? {};

  const headers = new Headers(rest.headers);
  let body = rest.body;

  if (json !== undefined) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(json);
  }

  const response = await fetch(url, {
    cache: "no-store",
    ...rest,
    headers,
    body,
  });

  const data = (await safeJSON(response)) as T | null;
  return { response, data };
}

/**
 * Convenience for mutations (POST/PUT/DELETE/PATCH).
 * Throws on non-ok response with extracted error message.
 */
export async function apiMutate<T = unknown>(
  url: string,
  method: "POST" | "PUT" | "DELETE" | "PATCH",
  body?: unknown,
): Promise<T> {
  const headers: HeadersInit = {};
  let serializedBody: BodyInit | undefined;

  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    serializedBody = JSON.stringify(body);
  }

  const response = await fetch(url, {
    method,
    headers,
    body: serializedBody,
    cache: "no-store",
  });

  const data = await safeJSON(response);

  if (!response.ok) {
    throw new Error(extractError(data, `Request failed: ${response.status}`));
  }

  return data as T;
}
