import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

type Params = {
  params: Promise<{
    id: string;
  }>;
};

export async function POST(request: Request, { params }: Params) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { id } = await params;
  const stackID = encodeURIComponent(id ?? "");

  let body: string | undefined;
  try {
    body = await request.text();
  } catch {
    body = undefined;
  }

  try {
    const response = await fetch(`${base.api}/api/v1/docker/stacks/${stackID}/action`, {
      method: "POST",
      cache: "no-store",
      headers: {
        ...authHeaders,
        "Content-Type": "application/json",
      },
      body,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        payload ?? { error: "failed to execute docker stack action" },
        { status: response.status }
      );
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "request failed" },
      { status: 502 }
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
