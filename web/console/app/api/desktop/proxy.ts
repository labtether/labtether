import { NextResponse } from "next/server";

import { upstreamErrorPayload } from "../../../lib/backend";
import {
  readBoundedRequestBody,
  RequestBodyTooLargeError,
} from "../../../lib/boundedBody";
import { markResponseNoStore } from "../../../lib/noStoreResponse";

export const maxDesktopTicketRequestBodyBytes = 4096;
export const desktopUpstreamTimeoutMs = 15_000;

export function desktopProxyJSON(payload: unknown, status = 200): Response {
  return markResponseNoStore(NextResponse.json(payload, { status }));
}

export function validDesktopResourceID(value: string): boolean {
  const normalized = value.trim();
  return (
    normalized.length > 0 &&
    normalized.length <= 255 &&
    !/[\u0000-\u001f\u007f-\u009f/\\]/u.test(normalized)
  );
}

export async function readDesktopSessionIDRequest(
  request: Request,
): Promise<
  | { sessionID: string }
  | { error: "invalid JSON payload" | "invalid sessionId" | "request body too large"; status: 400 | 413 }
> {
  let raw: ArrayBuffer;
  try {
    raw = await readBoundedRequestBody(
      request,
      maxDesktopTicketRequestBodyBytes,
    );
  } catch (error) {
    if (error instanceof RequestBodyTooLargeError) {
      return { error: "request body too large", status: 413 };
    }
    return { error: "invalid JSON payload", status: 400 };
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(raw));
  } catch {
    return { error: "invalid JSON payload", status: 400 };
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    return { error: "invalid JSON payload", status: 400 };
  }
  const sessionID =
    typeof (parsed as Record<string, unknown>).sessionId === "string"
      ? (parsed as Record<string, string>).sessionId.trim()
      : "";
  if (!validDesktopResourceID(sessionID)) {
    return { error: "invalid sessionId", status: 400 };
  }
  return { sessionID };
}

export async function safeDesktopResponseJSON(
  response: Response,
): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export function desktopUpstreamError(
  response: Response,
  payload: unknown,
  fallback: string,
): Response {
  return desktopProxyJSON(
    upstreamErrorPayload(response.status, payload, fallback),
    response.status,
  );
}
