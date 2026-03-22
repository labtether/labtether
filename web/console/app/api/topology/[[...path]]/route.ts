import { NextRequest, NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

/**
 * Proxy all /api/topology/* requests to the hub's /api/v2/topology/* endpoint.
 * Supports GET (query/fetch), POST (create), PUT (update), DELETE (remove).
 */
async function proxyToHub(request: NextRequest, { params }: { params: Promise<{ path?: string[] }> }) {
  const { path } = await params;
  const subPath = path ? path.map(encodeURIComponent).join("/") : "";
  const searchParams = request.nextUrl.searchParams.toString();
  const query = searchParams ? `?${searchParams}` : "";

  try {
    const base = await resolvedBackendBaseURLs();
    const url = `${base.api}/api/v2/topology${subPath ? `/${subPath}` : ""}${query}`;

    const authHeaders = backendAuthHeadersWithCookie(request);
    const headers: Record<string, string> = { ...authHeaders };

    const contentType = request.headers.get("content-type");
    if (contentType) {
      headers["Content-Type"] = contentType;
    }

    const fetchOptions: RequestInit = {
      method: request.method,
      headers,
    };

    if (request.method === "POST" || request.method === "PUT") {
      fetchOptions.body = request.body;
      fetchOptions.duplex = "half";
    }

    const response = await fetch(url, fetchOptions);

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
    const detail = error instanceof Error ? error.message : "topology operation failed";
    return NextResponse.json({ error: detail.slice(0, 240) }, { status: 502 });
  }
}

export const GET = proxyToHub;
export const POST = proxyToHub;
export const PUT = proxyToHub;
export const DELETE = proxyToHub;
