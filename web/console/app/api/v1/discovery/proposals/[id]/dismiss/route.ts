import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../lib/backend";

export const dynamic = "force-dynamic";

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export async function POST(request: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const base = await resolvedBackendBaseURLs();
    const body = await request.text();
    const response = await fetch(`${base.api}/discovery/proposals/${id}/dismiss`, {
      method: "POST",
      headers: { ...backendAuthHeadersWithCookie(request), "Content-Type": "application/json" },
      body,
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to dismiss proposal" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to dismiss proposal" },
      { status: 502 },
    );
  }
}
