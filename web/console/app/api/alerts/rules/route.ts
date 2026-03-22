import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const incoming = new URL(request.url);
  const backendURL = new URL(`${base.api}/alerts/rules`);
  for (const [key, value] of incoming.searchParams.entries()) {
    backendURL.searchParams.set(key, value);
  }
  try {
    const response = await fetch(backendURL.toString(), { cache: "no-store", headers: authHeaders });
    const payload = await safeJSON(response);
    if (!response.ok) return NextResponse.json(payload ?? { error: "failed to load alert rules" }, { status: response.status });
    return NextResponse.json(payload ?? []);
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "backend error" }, { status: 502 });
  }
}

export async function POST(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const body = await request.json();
    const response = await fetch(`${base.api}/alerts/rules`, {
      method: "POST",
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
    const payload = await safeJSON(response);
    if (!response.ok) return NextResponse.json(payload ?? { error: "failed to create rule" }, { status: response.status });
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "backend error" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}
