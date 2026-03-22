import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";

export const dynamic = "force-dynamic";

function backendPath(baseURL: string, channelId: string): string {
  return `${baseURL}/notifications/channels/${encodeURIComponent(channelId)}`;
}

export async function GET(request: Request, context: { params: Promise<{ channelId: string }> }) {
  const { channelId } = await context.params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(backendPath(base.api, channelId), { cache: "no-store", headers: authHeaders });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to load notification channel" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "backend error" },
      { status: 502 },
    );
  }
}

export async function PATCH(request: Request, context: { params: Promise<{ channelId: string }> }) {
  return mutateChannel(request, context, "PATCH");
}

export async function PUT(request: Request, context: { params: Promise<{ channelId: string }> }) {
  return mutateChannel(request, context, "PUT");
}

export async function DELETE(request: Request, context: { params: Promise<{ channelId: string }> }) {
  const { channelId } = await context.params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(backendPath(base.api, channelId), {
      method: "DELETE",
      headers: authHeaders,
      cache: "no-store",
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to delete notification channel" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "backend error" },
      { status: 502 },
    );
  }
}

async function mutateChannel(
  request: Request,
  context: { params: Promise<{ channelId: string }> },
  method: "PATCH" | "PUT",
) {
  const { channelId } = await context.params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const body = await request.json();
    const response = await fetch(backendPath(base.api, channelId), {
      method,
      headers: { ...authHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(payload ?? { error: "failed to update notification channel" }, { status: response.status });
    }
    return NextResponse.json(payload ?? {});
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "backend error" },
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
