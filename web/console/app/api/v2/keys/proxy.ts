import { NextResponse } from "next/server";

import { upstreamErrorPayload } from "../../../../lib/backend";
import { readBoundedRequestBody, RequestBodyTooLargeError } from "../../../../lib/boundedBody";
import { markResponseNoStore } from "../../../../lib/noStoreResponse";

export const maxAPIKeyRequestBodyBytes = 128 * 1024;

class InvalidAPIKeyRequestBodyError extends Error {
  constructor() {
    super("invalid JSON request body");
    this.name = "InvalidAPIKeyRequestBodyError";
  }
}

export async function readAPIKeyRequestJSON(request: Request): Promise<Record<string, unknown>> {
  let raw: ArrayBuffer;
  try {
    raw = await readBoundedRequestBody(request, maxAPIKeyRequestBodyBytes);
  } catch (error) {
    if (error instanceof RequestBodyTooLargeError) throw error;
    throw new InvalidAPIKeyRequestBodyError();
  }

  try {
    const text = new TextDecoder("utf-8", { fatal: true }).decode(raw);
    const parsed: unknown = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      throw new InvalidAPIKeyRequestBodyError();
    }
    return parsed as Record<string, unknown>;
  } catch (error) {
    if (error instanceof InvalidAPIKeyRequestBodyError) throw error;
    throw new InvalidAPIKeyRequestBodyError();
  }
}

export function apiKeyRequestErrorResponse(error: unknown): Response | null {
  if (error instanceof RequestBodyTooLargeError) {
    return apiKeyJSONResponse({ error: "request body too large" }, 413);
  }
  if (error instanceof InvalidAPIKeyRequestBodyError) {
    return apiKeyJSONResponse({ error: "invalid JSON request body" }, 400);
  }
  return null;
}

export function apiKeyJSONResponse(payload: unknown, status = 200): Response {
  return markResponseNoStore(NextResponse.json(payload, { status }));
}

export async function safeAPIKeyResponseJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export function apiKeyUpstreamErrorResponse(response: Response, payload: unknown, fallback: string): Response {
  return apiKeyJSONResponse(upstreamErrorPayload(response.status, payload, fallback), response.status);
}

export function apiKeyBackendUnavailableResponse(): Response {
  return apiKeyJSONResponse({ error: "API key endpoint unavailable" }, 502);
}

export function validAPIKeyID(value: string): boolean {
  const trimmed = value.trim();
  return trimmed.length > 0
    && trimmed.length <= 255
    && !/[\u0000-\u001f\u007f/\\]/.test(trimmed);
}
