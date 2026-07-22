import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";
import { readBoundedRequestBody, RequestBodyTooLargeError } from "../../../../../../lib/boundedBody";
import { isMutationRequestOriginAllowed } from "../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

const allowedResources = new Set(["overview", "containers", "stacks", "images", "volumes", "networks"]);
const maxRequestBodyBytes = 1024 * 1024;

type RouteContext = {
  params: Promise<{ assetId: string; path: string[] }>;
};

function isSafePathSegment(value: string): boolean {
  return value !== "."
    && value !== ".."
    && !value.includes("/")
    && !value.includes("\\")
    && !/[\u0000-\u001f\u007f]/.test(value);
}

async function proxyPortainerAsset(request: Request, { params }: RouteContext) {
  if (request.method !== "GET" && !isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const { assetId, path } = await params;
  const cleanAssetID = assetId.trim();
  const cleanPath = Array.isArray(path) ? path.map((segment) => segment.trim()).filter(Boolean) : [];
  if (
    !cleanAssetID
    || !isSafePathSegment(cleanAssetID)
    || cleanPath.length === 0
    || !cleanPath.every(isSafePathSegment)
    || !allowedResources.has(cleanPath[0])
    || cleanPath.length > 4
  ) {
    return NextResponse.json({ error: "not found" }, { status: 404 });
  }
  if (cleanPath[0] === "containers" && cleanPath[2] === "exec") {
    return NextResponse.json({ error: "not found" }, { status: 404 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const encodedPath = cleanPath.map(encodeURIComponent).join("/");
    const query = new URL(request.url).search;
    const headers: Record<string, string> = {
      ...backendAuthHeadersWithCookie(request),
    };
    const contentType = request.headers.get("content-type");
    if (contentType) headers["Content-Type"] = contentType;

    const init: RequestInit = {
      method: request.method,
      cache: "no-store",
      headers,
      signal: request.signal,
    };
    if (request.method !== "GET" && request.method !== "HEAD") {
      try {
        init.body = await readBoundedRequestBody(request, maxRequestBodyBytes);
      } catch (error) {
        if (error instanceof RequestBodyTooLargeError) {
          return NextResponse.json({ error: "request body too large" }, { status: 413 });
        }
        return NextResponse.json({ error: "invalid request body" }, { status: 400 });
      }
    }

    const response = await fetch(
      `${base.api}/portainer/assets/${encodeURIComponent(cleanAssetID)}/${encodedPath}${query}`,
      init,
    );
    const responseHeaders = new Headers();
    const responseContentType = response.headers.get("content-type");
    if (responseContentType) responseHeaders.set("content-type", responseContentType);
    const responseBody = response.status === 204 || response.status === 304 ? null : response.body;
    return new Response(responseBody, { status: response.status, headers: responseHeaders });
  } catch {
    return NextResponse.json({ error: "portainer endpoint unavailable" }, { status: 502 });
  }
}

export const GET = proxyPortainerAsset;
export const POST = proxyPortainerAsset;
export const PUT = proxyPortainerAsset;
export const PATCH = proxyPortainerAsset;
export const DELETE = proxyPortainerAsset;
