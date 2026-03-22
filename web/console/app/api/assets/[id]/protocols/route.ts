import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function GET(request: Request, context: RouteContext) {
  try {
    const { id } = await context.params;
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/assets/${encodeURIComponent(id)}/protocols`, {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to load protocols" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load protocols" },
      { status: 502 },
    );
  }
}

export async function POST(request: Request, context: RouteContext) {
  try {
    const { id } = await context.params;
    const body = await request.json();
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/assets/${encodeURIComponent(id)}/protocols`, {
      method: "POST",
      cache: "no-store",
      headers: {
        ...backendAuthHeadersWithCookie(request),
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to create protocol" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to create protocol" },
      { status: 502 },
    );
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
