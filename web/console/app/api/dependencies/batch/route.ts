import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const incoming = new URL(request.url);
    const backendURL = new URL(`${base.api}/dependencies/batch`);

    for (const [key, value] of incoming.searchParams.entries()) {
      backendURL.searchParams.append(key, value);
    }

    const response = await fetch(backendURL.toString(), {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to load dependencies" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load dependencies" },
      { status: 502 },
    );
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
