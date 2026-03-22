import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";

export async function GET(request: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const base = await resolvedBackendBaseURLs();
    const res = await fetch(
      `${base.api}/assets/${encodeURIComponent(id)}/desktop/credentials`,
      { cache: "no-store", headers: { ...backendAuthHeadersWithCookie(request) } }
    );
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "failed to fetch desktop credentials" }, { status: 500 });
  }
}

export async function POST(request: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const body = await request.json();
    const base = await resolvedBackendBaseURLs();
    const res = await fetch(
      `${base.api}/assets/${encodeURIComponent(id)}/desktop/credentials`,
      {
        method: "POST",
        cache: "no-store",
        headers: { "Content-Type": "application/json", ...backendAuthHeadersWithCookie(request) },
        body: JSON.stringify(body),
      }
    );
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "failed to save desktop credentials" }, { status: 500 });
  }
}

export async function DELETE(request: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const base = await resolvedBackendBaseURLs();
    const res = await fetch(
      `${base.api}/assets/${encodeURIComponent(id)}/desktop/credentials`,
      {
        method: "DELETE",
        cache: "no-store",
        headers: { ...backendAuthHeadersWithCookie(request) },
      }
    );
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "failed to delete desktop credentials" }, { status: 500 });
  }
}
