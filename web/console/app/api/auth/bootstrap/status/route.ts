import { NextResponse } from "next/server";
import { backendAuthHeaders, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET() {
  const base = await resolvedBackendBaseURLs();

  try {
    const response = await fetch(`${base.api}/auth/bootstrap/status`, {
      cache: "no-store",
      headers: backendAuthHeaders(),
    });

    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "bootstrap status unavailable" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "bootstrap status unavailable" },
      { status: 502 },
    );
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}
