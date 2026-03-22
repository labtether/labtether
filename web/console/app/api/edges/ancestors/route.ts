import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

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
    const backendURL = new URL(`${base.api}/edges/ancestors`);
    for (const [key, value] of incoming.searchParams.entries()) {
      backendURL.searchParams.append(key, value);
    }
    const response = await fetch(backendURL.toString(), {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to load edge ancestors" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load edge ancestors" },
      { status: 502 },
    );
  }
}
