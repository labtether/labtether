import { NextResponse } from "next/server";

import {
  backendAuthHeadersWithCookie,
  backendBaseURLs,
  resolvedBackendBaseURLs,
} from "../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const authHeaders = backendAuthHeadersWithCookie(request);

  const incoming = new URL(request.url);

  try {
    const response = await fetchGroupsBackend(request, "/groups", {
      cache: "no-store",
      headers: authHeaders,
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to list groups" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to query backend groups endpoint" },
      { status: 502 },
    );
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  try {
    const payload = await request.json();

    const response = await fetchGroupsBackend(request, "/groups", {
      method: "POST",
      cache: "no-store",
      headers: {
        ...backendAuthHeadersWithCookie(request),
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    });

    const responsePayload = await safeJSON(response);
    return NextResponse.json(responsePayload ?? { error: "failed to create group" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to create group" },
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

async function fetchGroupsBackend(request: Request, path: string, init: RequestInit): Promise<Response> {
  const resolved = await resolvedBackendBaseURLs();
  const baseline = backendBaseURLs();
  const origins = [resolved.api, baseline.api].filter(
    (value, index, items) => value && items.indexOf(value) === index,
  );

  let lastError: unknown = null;
  for (const origin of origins) {
    try {
      const backendURL = new URL(path, `${origin}/`);
      const incoming = new URL(request.url);
      for (const [key, value] of incoming.searchParams.entries()) {
        backendURL.searchParams.set(key, value);
      }
      return await fetch(backendURL.toString(), init);
    } catch (error) {
      lastError = error;
    }
  }

  throw lastError instanceof Error ? lastError : new Error("failed to reach backend groups endpoint");
}
