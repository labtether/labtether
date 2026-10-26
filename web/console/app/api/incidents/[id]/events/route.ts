import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { decodeIncidentTimeline } from "../../../../lib/incidentTimeline";

export const dynamic = "force-dynamic";

export async function GET(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const query = new URLSearchParams();
    const input = new URL(request.url).searchParams;
    for (const key of ["limit", "cursor"]) {
      const value = input.get(key);
      if (value !== null) query.set(key, value);
    }
    const response = await fetch(`${base.api}/incidents/${encodeURIComponent(id)}/timeline?${query}`, {
      cache: "no-store", headers: authHeaders, signal: request.signal,
    });
    const payload = await safeJSON(response);
    if (!response.ok) return NextResponse.json(payload ?? { error: "failed to load events" }, { status: response.status });
    return NextResponse.json(decodeIncidentTimeline(payload, id));
  } catch {
    return NextResponse.json({ error: "failed to load incident timeline" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}
