import { NextResponse } from "next/server";

import { readBoundedRequestBody, RequestBodyTooLargeError } from "../../../lib/boundedBody";
import { markResponseNoStore } from "../../../lib/noStoreResponse";

// The backend permits a 64 KiB config object. Leave bounded room for the
// surrounding channel name/type/enabled JSON envelope and escaping overhead.
export const maxNotificationRequestBodyBytes = 96 * 1024;

export class InvalidNotificationRequestBodyError extends Error {
  constructor() {
    super("invalid JSON request body");
    this.name = "InvalidNotificationRequestBodyError";
  }
}

export async function readNotificationRequestJSON(request: Request): Promise<Record<string, unknown>> {
  let raw: ArrayBuffer;
  try {
    raw = await readBoundedRequestBody(request, maxNotificationRequestBodyBytes);
  } catch (error) {
    if (error instanceof RequestBodyTooLargeError) throw error;
    throw new InvalidNotificationRequestBodyError();
  }

  try {
    const text = new TextDecoder("utf-8", { fatal: true }).decode(raw);
    const value: unknown = JSON.parse(text);
    if (value === null || typeof value !== "object" || Array.isArray(value)) {
      throw new InvalidNotificationRequestBodyError();
    }
    return value as Record<string, unknown>;
  } catch (error) {
    if (error instanceof InvalidNotificationRequestBodyError) throw error;
    throw new InvalidNotificationRequestBodyError();
  }
}

export function notificationRequestErrorResponse(error: unknown): Response | null {
  if (error instanceof RequestBodyTooLargeError) {
    return notificationJSONResponse({ error: "request body too large" }, 413);
  }
  if (error instanceof InvalidNotificationRequestBodyError) {
    return notificationJSONResponse({ error: "invalid JSON request body" }, 400);
  }
  return null;
}

export function notificationJSONResponse(payload: unknown, status = 200): Response {
  return markResponseNoStore(NextResponse.json(payload, { status }));
}

export async function safeNotificationResponseJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export function notificationBackendUnavailableResponse(testRequest = false): Response {
  return notificationJSONResponse(
    testRequest
      ? { success: false, error: "notification backend unavailable" }
      : { error: "notification backend unavailable" },
    502,
  );
}
