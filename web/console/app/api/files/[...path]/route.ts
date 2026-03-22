import { NextRequest, NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

/**
 * Proxy all /api/files/* requests to the hub's /files/* endpoint.
 * Supports GET (list, download), POST (upload, mkdir), DELETE.
 */
async function proxyToHub(request: NextRequest, { params }: { params: Promise<{ path: string[] }> }) {
  const { path } = await params;
  // Encode each path segment individually to prevent path traversal
  const subPath = path.map(encodeURIComponent).join("/");
  const searchParams = request.nextUrl.searchParams.toString();
  const query = searchParams ? `?${searchParams}` : "";

  try {
    const base = await resolvedBackendBaseURLs();
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

    // For JSON responses, pass through.
    const data = await response.json();
    return NextResponse.json(data, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "file operation failed" },
      { status: 502 }
    );
  }
}

export const GET = proxyToHub;
export const POST = proxyToHub;
export const DELETE = proxyToHub;
