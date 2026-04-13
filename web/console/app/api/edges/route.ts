import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const incoming = new URL(request.url);
    const backendURL = new URL(`${base.api}/edges`);
    for (const [key, value] of incoming.searchParams.entries()) {
      backendURL.searchParams.append(key, value);
    }
    const response = await fetch(backendURL.toString(), {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to list edges" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to list edges" },
      { status: 502 },
    );
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const body = await request.text();
    const response = await fetch(`${base.api}/edges`, {
      method: "POST",
      headers: { ...backendAuthHeadersWithCookie(request), "Content-Type": "application/json" },
      body,
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to create edge" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to create edge" },
      { status: 502 },
    );
  }
}
