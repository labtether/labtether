import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

const backendPath = (baseURL: string, ruleId: string) => `${baseURL}/alerts/rules/${encodeURIComponent(ruleId)}`;

/** Changes some of a rule's fields (status turns it on or off). The hub keeps the fields the body leaves out. */
export async function PATCH(request: Request, { params }: { params: Promise<{ ruleId: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const body: unknown = await request.json().catch(() => null);
  if (!body || typeof body !== "object" || Array.isArray(body)) {
    return NextResponse.json({ error: "invalid alert rule payload" }, { status: 400 });
  }

  const { ruleId } = await params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(backendPath(base.api, ruleId), {
      method: "PATCH",
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
    const payload = await safeJSON(response);
    if (!response.ok) return NextResponse.json(payload ?? { error: "failed to update rule" }, { status: response.status });
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "backend error" }, { status: 502 });
  }
}

export async function DELETE(request: Request, { params }: { params: Promise<{ ruleId: string }> }) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const { ruleId } = await params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(backendPath(base.api, ruleId), {
      method: "DELETE",
      headers: authHeaders,
      cache: "no-store",
    });
    if (!response.ok) {
      const payload = await safeJSON(response);
      return NextResponse.json(payload ?? { error: "failed to delete rule" }, { status: response.status });
    }
    return NextResponse.json({ ok: true });
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "backend error" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}
