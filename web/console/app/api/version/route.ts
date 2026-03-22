import { NextResponse } from "next/server";
import { backendAuthHeaders, backendBaseURLs } from "../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET() {
  const { api } = backendBaseURLs();
  const authHeaders = backendAuthHeaders();

  try {
    const res = await fetch(`${api}/version`, {
      cache: "no-store",
      headers: authHeaders,
    });
    if (!res.ok) {
      return NextResponse.json({ error: "version endpoint unavailable" }, { status: res.status });
    }
    const payload = await res.json();
    return NextResponse.json(payload);
  } catch {
    return NextResponse.json({ error: "version endpoint unavailable" }, { status: 502 });
  }
}
