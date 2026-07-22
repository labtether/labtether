import { NextResponse } from "next/server";

import { upstreamErrorPayload } from "../../../../lib/backend";
import { readBoundedRequestBody, RequestBodyTooLargeError } from "../../../../lib/boundedBody";
import { markResponseNoStore } from "../../../../lib/noStoreResponse";

export const maxSavedActionRequestBodyBytes = 256 * 1024;
export const maxSavedActionEmptyMutationBodyBytes = 1024;

class InvalidSavedActionRequestBodyError extends Error {
  constructor() {
    super("invalid JSON request body");
    this.name = "InvalidSavedActionRequestBodyError";
  }
}

export async function readSavedActionRequestJSON(request: Request): Promise<Record<string, unknown>> {
  let raw: ArrayBuffer;
  try {
    raw = await readBoundedRequestBody(request, maxSavedActionRequestBodyBytes);
  } catch (error) {
    if (error instanceof RequestBodyTooLargeError) throw error;
    throw new InvalidSavedActionRequestBodyError();
  }

  try {
    const text = new TextDecoder("utf-8", { fatal: true }).decode(raw);
    const parsed: unknown = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      throw new InvalidSavedActionRequestBodyError();
    }
    return parsed as Record<string, unknown>;
  } catch (error) {
    if (error instanceof InvalidSavedActionRequestBodyError) throw error;
    throw new InvalidSavedActionRequestBodyError();
  }
}

export async function validateSavedActionEmptyMutationBody(request: Request): Promise<void> {
  let raw: ArrayBuffer;
  try {
    raw = await readBoundedRequestBody(request, maxSavedActionEmptyMutationBodyBytes);
  } catch (error) {
    if (error instanceof RequestBodyTooLargeError) throw error;
    throw new InvalidSavedActionRequestBodyError();
  }
  try {
    const text = new TextDecoder("utf-8", { fatal: true }).decode(raw).trim();
    if (text === "") return;
    const parsed: unknown = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed) || Object.keys(parsed).length !== 0) {
      throw new InvalidSavedActionRequestBodyError();
    }
  } catch (error) {
    if (error instanceof InvalidSavedActionRequestBodyError) throw error;
    throw new InvalidSavedActionRequestBodyError();
  }
}

export function savedActionRequestErrorResponse(error: unknown): Response | null {
  if (error instanceof RequestBodyTooLargeError) {
    return savedActionJSONResponse({ error: "request body too large" }, 413);
  }
  if (error instanceof InvalidSavedActionRequestBodyError) {
    return savedActionJSONResponse({ error: "invalid JSON request body" }, 400);
  }
  return null;
}

export function savedActionJSONResponse(payload: unknown, status = 200): Response {
  return markResponseNoStore(NextResponse.json(payload, { status }));
}

export async function safeSavedActionResponseJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export function savedActionUpstreamErrorResponse(response: Response, payload: unknown, fallback: string): Response {
  return savedActionJSONResponse(upstreamErrorPayload(response.status, payload, fallback), response.status);
}

export function savedActionBackendUnavailableResponse(): Response {
  return savedActionJSONResponse({ error: "saved actions endpoint unavailable" }, 502);
}

export function validSavedActionID(value: string): boolean {
  return value.length > 0
    && value.length <= 255
    && value === value.trim()
    && !/[\u0000-\u001f\u007f/\\]/.test(value);
}
