import { NextResponse } from "next/server";
import { backendBaseURLs, backendAuthHeadersWithCookie } from "../../../../../lib/backend";

export async function DELETE(request: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const base = backendBaseURLs();
    const res = await fetch(`${base.api}/settings/enrollment/${encodeURIComponent(id)}`, {
      method: "DELETE",
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "failed to revoke token" }, { status: 500 });
  }
}
