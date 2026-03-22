import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";

export const dynamic = "force-dynamic";

type Params = {
  params: Promise<{
    id: string;
    sub: string;
  }>;
};

export async function GET(request: Request, { params }: Params) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { id, sub } = await params;
  const containerID = encodeURIComponent(id ?? "");
  const subPath = encodeURIComponent(sub ?? "");

  // Forward query parameters (e.g. ?tail=N for logs)
  const { searchParams } = new URL(request.url);
  const qs = searchParams.toString();
  const backendURL = `${base.api}/api/v1/docker/containers/${containerID}/${subPath}${qs ? `?${qs}` : ""}`;

  try {
    const response = await fetch(backendURL, {
      cache: "no-store",
      headers: authHeaders,
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        payload ?? { error: `failed to load docker container ${sub}` },
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

export async function POST(request: Request, { params }: Params) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const { id, sub } = await params;
  const containerID = encodeURIComponent(id ?? "");
  const subPath = encodeURIComponent(sub ?? "");

  let body: string | undefined;
  try {
    body = await request.text();
  } catch {
    body = undefined;
  }

  try {
    const response = await fetch(`${base.api}/api/v1/docker/containers/${containerID}/${subPath}`, {
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
        payload ?? { error: `failed to execute docker container ${sub}` },
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
