import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../../lib/backend";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ id: string; protocol: string }>;
};

export async function POST(request: Request, context: RouteContext) {
  try {
    const { id, protocol } = await context.params;
    const body = await request.json();
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(
      `${base.api}/assets/${encodeURIComponent(id)}/protocols/${encodeURIComponent(protocol)}/test`,
      {
        method: "POST",
        cache: "no-store",
        headers: {
          ...backendAuthHeadersWithCookie(request),
          "Content-Type": "application/json",
        },
        body: JSON.stringify(body),
      },
    );
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to test protocol" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to test protocol" },
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
