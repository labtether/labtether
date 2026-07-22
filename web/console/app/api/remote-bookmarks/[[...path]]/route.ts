import { NextRequest, NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

/**
 * Proxy all /api/remote-bookmarks/* requests to the hub's /api/v1/remote-bookmarks/* endpoint.
 * Supports GET (list), POST (create), PUT (update), DELETE (remove).
 */
async function proxyToHub(request: NextRequest, { params }: { params: Promise<{ path?: string[] }> }) {
  if (
    request.method !== "GET"
    && request.method !== "HEAD"
    && !isMutationRequestOriginAllowed(request)
  ) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const { path } = await params;
  // Encode each path segment individually to prevent path traversal
  const subPath = path ? path.map(encodeURIComponent).join("/") : "";
  const searchParams = request.nextUrl.searchParams.toString();
  const query = searchParams ? `?${searchParams}` : "";

  try {
    const base = await resolvedBackendBaseURLs();
    const url = `${base.api}/api/v1/remote-bookmarks/${subPath}${query}`;

    const authHeaders = backendAuthHeadersWithCookie(request);
    const headers: Record<string, string> = {
      ...authHeaders,
    };

    // Forward content-type for uploads.
    const contentType = request.headers.get("content-type");
    if (contentType) {
      headers["Content-Type"] = contentType;
    }

    const fetchOptions: RequestInit = {
      method: request.method,
      headers,
      cache: "no-store",
    };

    // Forward body for POST/PUT.
    if (request.method === "POST" || request.method === "PUT") {
      fetchOptions.body = request.body;
      fetchOptions.duplex = "half";
    }

    const response = await fetch(url, fetchOptions);

    if (response.status === 204 || response.status === 304) {
      return new NextResponse(null, { status: response.status });
    }

    // For downloads, stream the response body directly.
    const disposition = response.headers.get("content-disposition");
    if (disposition) {
      return new NextResponse(response.body, {
        status: response.status,
        headers: {
          "Content-Type": response.headers.get("content-type") || "application/octet-stream",
          "Content-Disposition": disposition,
        },
      });
    }

    // For JSON responses, pass through. Handle non-JSON gracefully.
    const text = await response.text();
    try {
      const data = JSON.parse(text);
      return NextResponse.json(data, {
        status: response.status,
        headers: subPath.endsWith("/credentials")
          ? { "Cache-Control": "no-store, private", Pragma: "no-cache" }
          : undefined,
      });
    } catch {
      return new NextResponse(text, {
        status: response.status,
        headers: { "Content-Type": response.headers.get("content-type") || "text/plain" },
      });
    }
  } catch (error) {
    const detail = error instanceof Error ? error.message : "remote-bookmark operation failed";
    return NextResponse.json(
      { error: detail.slice(0, 240) },
      { status: 502 }
    );
  }
}

export const GET = proxyToHub;
export const POST = proxyToHub;
export const PUT = proxyToHub;
export const DELETE = proxyToHub;
