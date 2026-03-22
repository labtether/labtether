import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/auth/logout`, {
      method: "POST",
      headers: authHeaders,
      cache: "no-store",
    });

    const res = NextResponse.json({ ok: response.ok }, { status: response.status });
    const setCookie = response.headers.get("set-cookie");
    if (setCookie) {
      res.headers.set("set-cookie", setCookie);
    }
    return res;
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "logout failed" },
      { status: 502 }
    );
  }
}
