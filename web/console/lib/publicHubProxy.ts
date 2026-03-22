import { NextRequest, NextResponse } from "next/server";
import { resolvedBackendBaseURLs } from "./backend";

function firstForwardedValue(raw: string | null): string {
  return raw?.split(",")[0]?.trim() ?? "";
}

function publicRequestHost(request: NextRequest): string {
  return firstForwardedValue(request.headers.get("x-forwarded-host"))
    || request.headers.get("host")?.trim()
    || request.nextUrl.host;
}

function publicRequestProto(request: NextRequest): string {
  return firstForwardedValue(request.headers.get("x-forwarded-proto"))
    || request.nextUrl.protocol.replace(":", "");
}

const MAX_PUBLIC_BODY_BYTES = 1 << 20; // 1 MiB — matches backend MaxJSONBodyBytes

export async function proxyPublicHubRequest(request: NextRequest, upstreamPath: string): Promise<NextResponse> {
  const base = await resolvedBackendBaseURLs();
  const upstreamURL = new URL(upstreamPath, `${base.api.replace(/\/+$/, "")}/`);
  upstreamURL.search = request.nextUrl.search;

  const headers = new Headers();
  const contentType = request.headers.get("content-type");
  const accept = request.headers.get("accept");
  const range = request.headers.get("range");
  const ifNoneMatch = request.headers.get("if-none-match");
  const ifModifiedSince = request.headers.get("if-modified-since");
  const host = publicRequestHost(request);
  const proto = publicRequestProto(request);

  if (contentType) {
    headers.set("Content-Type", contentType);
  }
  if (accept) {
    headers.set("Accept", accept);
  }
  if (range) {
    headers.set("Range", range);
  }
  if (ifNoneMatch) {
    headers.set("If-None-Match", ifNoneMatch);
  }
  if (ifModifiedSince) {
    headers.set("If-Modified-Since", ifModifiedSince);
  }

  // Preserve the public origin so generated URLs stay on the frontend/Tailscale
  // host instead of collapsing back to the internal backend address.
  headers.set("Host", host);
  headers.set("X-Forwarded-Host", host);
  headers.set("X-Forwarded-Proto", proto);

  const init: RequestInit = {
    method: request.method,
    headers,
    redirect: "manual",
  };

  if (request.method !== "GET" && request.method !== "HEAD") {
    const contentLength = Number(request.headers.get("content-length") ?? "0");
    if (contentLength > MAX_PUBLIC_BODY_BYTES) {
      return new NextResponse("request body too large", { status: 413 });
    }
    const body = await request.arrayBuffer();
    if (body.byteLength > MAX_PUBLIC_BODY_BYTES) {
      return new NextResponse("request body too large", { status: 413 });
    }
    if (body.byteLength > 0) {
      init.body = body;
    }
  }

  const response = await fetch(upstreamURL, init);
  const responseHeaders = new Headers(response.headers);
  responseHeaders.delete("content-encoding");
  responseHeaders.delete("content-length");
  responseHeaders.delete("transfer-encoding");
  responseHeaders.delete("connection");

  return new NextResponse(response.body, {
    status: response.status,
    headers: responseHeaders,
  });
}
