import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ id: string }>;
};

/** What removing a device also removes (Hub GET /assets/{id}/delete-preview). */
export async function GET(request: Request, context: RouteContext) {
  try {
    const { id } = await context.params;
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/assets/${encodeURIComponent(id)}/delete-preview`, {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const payload = await response.json().catch(() => null);
    return NextResponse.json(payload ?? { error: "failed to load removal preview" }, { status: response.status });
  } catch {
    return NextResponse.json({ error: "failed to load removal preview" }, { status: 502 });
  }
}
