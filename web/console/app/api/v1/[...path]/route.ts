import { NextRequest, NextResponse } from "next/server";
import { proxyPublicHubRequest } from "../../../../lib/publicHubProxy";

const allowedPublicHubPaths = new Set<string>([
  "enroll",
  "discover",
  "ca.crt",
  "tls/info",
  "agent/binary",
  "agent/releases/latest",
  "agent/install.sh",
  "agent/bootstrap.sh",
]);

function normalizedHubPath(path: string[]): string {
  return path.join("/").replace(/^\/+/, "").replace(/\/+$/, "");
}

async function handle(request: NextRequest, { params }: { params: Promise<{ path: string[] }> }): Promise<NextResponse> {
  const { path } = await params;
  const normalizedPath = normalizedHubPath(path);
  if (!allowedPublicHubPaths.has(normalizedPath)) {
    return NextResponse.json({ error: "not found" }, { status: 404 });
  }
  return proxyPublicHubRequest(request, `/api/v1/${normalizedPath}`);
}

export async function GET(request: NextRequest, context: { params: Promise<{ path: string[] }> }): Promise<NextResponse> {
  return handle(request, context);
}

export async function POST(request: NextRequest, context: { params: Promise<{ path: string[] }> }): Promise<NextResponse> {
  return handle(request, context);
}

export async function HEAD(request: NextRequest, context: { params: Promise<{ path: string[] }> }): Promise<NextResponse> {
  return handle(request, context);
}
