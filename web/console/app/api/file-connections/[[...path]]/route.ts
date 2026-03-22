import { NextRequest, NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

/**
 * Proxy all /api/file-connections/* requests to the hub's /file-connections/* endpoint.
 * Supports GET (list/status), POST (create), PUT (update), DELETE (remove).
 */
async function proxyToHub(request: NextRequest, { params }: { params: Promise<{ path?: string[] }> }) {
  const { path } = await params;
  // Encode each path segment individually to prevent path traversal
  const subPath = path ? path.map(encodeURIComponent).join("/") : "";
  const searchParams = request.nextUrl.searchParams.toString();
  const query = searchParams ? `?${searchParams}` : "";

  try {
    const base = await resolvedBackendBaseURLs();
    const url = `${base.api}/api/v1/file-connections/${subPath}${query}`;

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
    };

    // Forward body for POST/PUT.
    if (request.method === "POST" || request.method === "PUT") {
      fetchOptions.body = request.body;
      fetchOptions.duplex = "half";
    }

    const response = await fetch(url, fetchOptions);

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
      return NextResponse.json(data, { status: response.status });
    } catch {
      return new NextResponse(text, {
        status: response.status,
        headers: { "Content-Type": response.headers.get("content-type") || "text/plain" },
      });
    }
  } catch (error) {
    const detail = error instanceof Error ? error.message : "file-connection operation failed";
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
