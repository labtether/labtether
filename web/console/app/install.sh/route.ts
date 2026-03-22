import { NextRequest, NextResponse } from "next/server";
import { proxyPublicHubRequest } from "../../lib/publicHubProxy";

export async function GET(request: NextRequest): Promise<NextResponse> {
  return proxyPublicHubRequest(request, "/install.sh");
}

export async function HEAD(request: NextRequest): Promise<NextResponse> {
  return proxyPublicHubRequest(request, "/install.sh");
}
