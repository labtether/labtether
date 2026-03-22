import { NextResponse } from "next/server";
import { backendBaseURLs, backendAuthHeadersWithCookie } from "../../../../lib/backend";

export async function GET(request: Request) {
  try {
    const base = backendBaseURLs();
    const res = await fetch(`${base.api}/settings/agent-tokens`, {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    if (!res.ok) {
      const payload = (await res.json().catch(() => null)) as { error?: string } | null;
      return NextResponse.json(payload ?? { error: "failed to fetch agent tokens" }, { status: res.status });
    }
    const data = await res.json();
    return NextResponse.json(data);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to fetch agent tokens" },
      { status: 502 }
    );
  }
}
