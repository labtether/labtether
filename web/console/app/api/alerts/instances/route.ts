import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const incoming = new URL(request.url);
  const backendURL = new URL(`${base.api}/alerts/instances`);
  for (const [key, value] of incoming.searchParams.entries()) {
    backendURL.searchParams.set(key, value);
  }
  try {
    const response = await fetch(backendURL.toString(), { cache: "no-store", headers: authHeaders });
    const payload = await safeJSON(response);
    if (!response.ok) return NextResponse.json(payload ?? { error: "failed to load alert instances" }, { status: response.status });
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
    const instanceId = typeof body.id === "string" ? body.id.trim() : "";
    if (!instanceId) {
      return NextResponse.json({ error: "instance id is required" }, { status: 400 });
    }
    const action = typeof body.action === "string" ? body.action : "";
    if (action !== "acknowledge" && action !== "resolve") {
      return NextResponse.json({ error: "action must be acknowledge or resolve" }, { status: 400 });
    }
    const backendAction = action === "acknowledge" ? "ack" : "resolve";
    const response = await fetch(`${base.api}/alerts/instances/${encodeURIComponent(instanceId)}/${backendAction}`, {
      method: "POST",
      headers: authHeaders,
      cache: "no-store",
    });
    const payload = await safeJSON(response);
    if (!response.ok) return NextResponse.json(payload ?? { error: "failed" }, { status: response.status });
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "backend error" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}
