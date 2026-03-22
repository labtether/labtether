import { NextResponse } from "next/server";
import { backendAuthHeaders, resolvedBackendBaseURLs, upstreamErrorPayload } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

function parseBootstrapRequest(raw: unknown): { username: string; password: string } | null {
  if (typeof raw !== "object" || raw === null) return null;
  const obj = raw as Record<string, unknown>;
  if (typeof obj.username !== "string" || !obj.username.trim()) return null;
  if (typeof obj.password !== "string" || !obj.password) return null;
  return { username: obj.username.trim(), password: obj.password };
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  try {
    const raw = await request.json();
    const body = parseBootstrapRequest(raw);
    if (!body) {
      return NextResponse.json({ error: "username and password are required" }, { status: 400 });
    }

    const response = await fetch(`${base.api}/auth/bootstrap`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...backendAuthHeaders() },
      body: JSON.stringify(body),
      cache: "no-store",
    });

    const payload = await safeJSON(response);
    const res = NextResponse.json(
      response.ok ? (payload ?? {}) : upstreamErrorPayload(response.status, payload, "setup failed"),
      { status: response.status },
    );

    const setCookie = response.headers.get("set-cookie");
    if (setCookie) {
      res.headers.set("set-cookie", setCookie);
    }

    return res;
  } catch {
    return NextResponse.json({ error: "setup endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}
