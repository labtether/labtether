import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}

function schedulePaginationQuery(request: Request): string | null {
  const input = new URL(request.url).searchParams;
  const output = new URLSearchParams();
  if (input.has("per_page") && input.has("page_size")) return null;

  for (const key of ["page", "per_page", "page_size"] as const) {
    const values = input.getAll(key);
    if (values.length > 1) return null;
    if (values.length === 0) continue;

    const value = values[0] ?? "";
    if (!/^[1-9]\d*$/.test(value)) return null;
    const parsed = Number(value);
    if (!Number.isSafeInteger(parsed) || (key !== "page" && parsed > 100)) return null;
    output.set(key, String(parsed));
  }

  const query = output.toString();
  return query ? `?${query}` : "";
}

export async function GET(request: Request) {
  const query = schedulePaginationQuery(request);
  if (query === null) {
    return NextResponse.json({ error: "invalid schedule pagination" }, { status: 400 });
  }
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(`${base.api}/api/v2/schedules${query}`, { cache: "no-store", headers: authHeaders });
    const payload = await safeJSON(response);
    if (!response.ok) return NextResponse.json(payload ?? { error: "failed to load schedules" }, { status: response.status });
    return NextResponse.json(payload ?? { schedules: [] });
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "backend error" }, { status: 502 });
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const body = await request.json();
    const response = await fetch(`${base.api}/api/v2/schedules`, {
      method: "POST",
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
    const payload = await safeJSON(response);
    if (!response.ok) return NextResponse.json(payload ?? { error: "failed to create schedule" }, { status: response.status });
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "backend error" }, { status: 502 });
  }
}
