import { NextResponse } from "next/server";
import { resolvedBackendBaseURLs, upstreamErrorPayload } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";
import { checkRateLimit } from "../../../../lib/rateLimit";

export const dynamic = "force-dynamic";

function parseLoginRequest(raw: unknown): { username: string; password: string } | null {
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

  const ip = request.headers.get("x-forwarded-for")?.split(",")[0]?.trim()
    || request.headers.get("x-real-ip")
    || "unknown";

  const { success, remaining, resetAt } = checkRateLimit(`login:${ip}`);
  if (!success) {
    return Response.json(
      { error: "Too many login attempts. Try again later." },
      {
        status: 429,
        headers: {
          "Retry-After": String(Math.ceil((resetAt - Date.now()) / 1000)),
          "X-RateLimit-Remaining": "0",
        },
      }
    );
  }

  const base = await resolvedBackendBaseURLs();
  try {
    const raw = await request.json();
    const body = parseLoginRequest(raw);
    if (!body) {
      return NextResponse.json({ error: "username and password are required" }, { status: 400 });
    }
    const response = await fetch(`${base.api}/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });

    const payload = await safeJSON(response);
    const res = NextResponse.json(
      response.ok ? (payload ?? {}) : upstreamErrorPayload(response.status, payload, "login failed"),
      { status: response.status },
    );

    // Forward set-cookie headers from backend (use getSetCookie to avoid
    // comma-joining that corrupts Expires dates in multi-cookie responses).
    for (const cookie of response.headers.getSetCookie()) {
      res.headers.append("set-cookie", cookie);
    }

    return res;
  } catch {
    return NextResponse.json({ error: "login endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try { return await response.json(); } catch { return null; }
}
