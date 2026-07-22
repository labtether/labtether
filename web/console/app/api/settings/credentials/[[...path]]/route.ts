import { NextResponse } from "next/server";

import {
  backendAuthHeadersWithCookie,
  resolvedBackendBaseURLs,
  upstreamErrorPayload,
} from "../../../../../lib/backend";
import {
  readBoundedRequestBody,
  RequestBodyTooLargeError,
} from "../../../../../lib/boundedBody";
import { markResponseNoStore } from "../../../../../lib/noStoreResponse";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export const maxCredentialRequestBodyBytes = 64 * 1024;
const maxCredentialIDBytes = 255;

type RouteContext = { params: Promise<{ path?: string[] }> };

function jsonResponse(payload: unknown, status: number): Response {
  return markResponseNoStore(NextResponse.json(payload, { status }));
}

function validCredentialID(value: string): boolean {
  return value.length > 0
    && value.length <= maxCredentialIDBytes
    && value !== "."
    && value !== ".."
    && !/[\u0000-\u001f\u007f/\\]/.test(value);
}

function credentialPath(method: string, segments: string[]): string | null {
  if (method === "GET" && segments.length === 0) return "";
  if (method === "POST" && segments.length === 0) return "";
  if ((method === "GET" || method === "DELETE") && segments.length === 1 && validCredentialID(segments[0])) {
    return `/${encodeURIComponent(segments[0])}`;
  }
  if (method === "POST" && segments.length === 2 && validCredentialID(segments[0]) && segments[1] === "rotate") {
    return `/${encodeURIComponent(segments[0])}/rotate`;
  }
  return null;
}

function boundedListQuery(request: Request, path: string): string {
  if (path !== "") return "";
  const raw = new URL(request.url).searchParams.get("limit") ?? "";
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed)) return "?limit=500";
  return `?limit=${Math.min(500, Math.max(1, parsed))}`;
}

async function responsePayload(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

async function proxyCredentialRequest(request: Request, context: RouteContext): Promise<Response> {
  if (request.method !== "GET" && !isMutationRequestOriginAllowed(request)) {
    return jsonResponse({ error: "forbidden origin" }, 403);
  }

  const { path = [] } = await context.params;
  const upstreamPath = credentialPath(request.method, path);
  if (upstreamPath === null) {
    return jsonResponse({ error: "credential profile path not found" }, 404);
  }

  let body: ArrayBuffer | undefined;
  if (request.method === "POST") {
    try {
      body = await readBoundedRequestBody(request, maxCredentialRequestBodyBytes);
    } catch (error) {
      if (error instanceof RequestBodyTooLargeError) {
        return jsonResponse({ error: "request body too large" }, 413);
      }
      return jsonResponse({ error: "invalid request body" }, 400);
    }
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const headers: Record<string, string> = {
      ...backendAuthHeadersWithCookie(request),
    };
    if (body) headers["Content-Type"] = "application/json";
    const response = await fetch(
      `${base.api}/api/v2/credentials/profiles${upstreamPath}${boundedListQuery(request, upstreamPath)}`,
      {
        method: request.method,
        headers,
        body: body ? new Uint8Array(body) : undefined,
        cache: "no-store",
        signal: AbortSignal.timeout(10_000),
      },
    );
    const payload = await responsePayload(response);
    if (!response.ok) {
      if (response.status >= 500) {
        return jsonResponse(
          upstreamErrorPayload(response.status, payload, "credential profile endpoint unavailable"),
          response.status,
        );
      }
      return jsonResponse(payload ?? { error: "credential profile request failed" }, response.status);
    }
    return jsonResponse(payload ?? {}, response.status);
  } catch {
    return jsonResponse({ error: "credential profile endpoint unavailable" }, 502);
  }
}

export const GET = proxyCredentialRequest;
export const POST = proxyCredentialRequest;
export const DELETE = proxyCredentialRequest;
