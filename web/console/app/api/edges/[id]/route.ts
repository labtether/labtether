import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export async function PUT(request: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const base = await resolvedBackendBaseURLs();
    const body = await request.text();
    const response = await fetch(`${base.api}/edges/${encodeURIComponent(id)}`, {
      method: "PUT",
      headers: { ...backendAuthHeadersWithCookie(request), "Content-Type": "application/json" },
      body,
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to update edge" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to update edge" },
      { status: 502 },
    );
  }
}

export async function PATCH(request: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const base = await resolvedBackendBaseURLs();
    const body = await request.text();
    const response = await fetch(`${base.api}/edges/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { ...backendAuthHeadersWithCookie(request), "Content-Type": "application/json" },
      body,
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to update edge" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to update edge" },
      { status: 502 },
    );
  }
}

export async function DELETE(request: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/edges/${encodeURIComponent(id)}`, {
      method: "DELETE",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? {}, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to delete edge" },
      { status: 502 },
    );
  }
}
