import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed, isProxyRequestAuthorized } from "../../../../lib/proxyAuth";

const forwardedResponseHeaders = [
  "accept-ranges",
  "cache-control",
  "content-disposition",
  "content-range",
  "content-type",
  "etag",
  "last-modified",
] as const;

type FileProxyDependencies = {
  resolveBaseURLs?: () => Promise<{ api: string }>;
  fetchUpstream?: typeof fetch;
};

export function authorizeFileProxyRequest(request: Request): Response | null {
  const pathname = new URL(request.url).pathname;
  if (!isProxyRequestAuthorized(pathname, request.headers)) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }
  return null;
}

/**
 * Proxy all /api/files/* requests to the hub's /files/* endpoint.
 * Supports GET (list, download), POST (upload, mkdir), DELETE.
 */
export async function proxyToHub(
  request: Request,
  { params }: { params: Promise<{ path: string[] }> },
  dependencies: FileProxyDependencies = {},
) {
  const authError = authorizeFileProxyRequest(request);
  if (authError) return authError;

  const { path } = await params;
  // Encode each path segment individually to prevent path traversal
  const subPath = path.map(encodeURIComponent).join("/");
  const searchParams = new URL(request.url).searchParams.toString();
  const query = searchParams ? `?${searchParams}` : "";

  try {
    const base = await (dependencies.resolveBaseURLs ?? resolvedBackendBaseURLs)();
    const url = `${base.api}/files/${subPath}${query}`;

    const authHeaders = backendAuthHeadersWithCookie(request);
    const headers: Record<string, string> = {
      ...authHeaders,
    };

    // Forward content-type for uploads.
    const contentType = request.headers.get("content-type");
    if (contentType) {
      headers["Content-Type"] = contentType;
    }
    const contentLength = request.headers.get("content-length");
    if (contentLength) {
      headers["Content-Length"] = contentLength;
    }
    const accept = request.headers.get("accept");
    if (accept) {
      headers.Accept = accept;
    }

    const fetchOptions: RequestInit = {
      method: request.method,
      headers,
      cache: "no-store",
      signal: request.signal,
    };

    // Node fetch requires duplex="half" when forwarding a ReadableStream.
    if (request.method === "POST" || request.method === "PUT" || request.method === "PATCH") {
      fetchOptions.body = request.body;
      fetchOptions.duplex = "half";
    }

    const response = await (dependencies.fetchUpstream ?? fetch)(url, fetchOptions);

    const responseHeaders = new Headers();
    for (const header of forwardedResponseHeaders) {
      const value = response.headers.get(header);
      if (value) responseHeaders.set(header, value);
    }
    // Node's fetch transparently decodes gzip/br bodies. Forwarding the
    // upstream encoding or byte length would make the browser try to decode
    // the already-decoded stream (ERR_CONTENT_DECODING_FAILED).
    const responseBody = response.status === 204 || response.status === 304 ? null : response.body;
    return new Response(responseBody, { status: response.status, headers: responseHeaders });
  } catch {
    return NextResponse.json({ error: "file operation failed" }, { status: 502 });
  }
}

export const GET = proxyToHub;
export const POST = proxyToHub;
export const DELETE = proxyToHub;
