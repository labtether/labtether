import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function DELETE(request: Request, { params }: { params: Promise<{ ruleId: string }> }) {
  const { ruleId } = await params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(`${base.api}/alerts/rules/${encodeURIComponent(ruleId)}`, {
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
