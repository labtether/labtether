import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function POST(request: Request, context: { params: Promise<{ channelId: string }> }) {
  const { channelId } = await context.params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  try {
    const response = await fetch(
      `${base.api}/notifications/channels/${encodeURIComponent(channelId)}/test`,
      {
        method: "POST",
        headers: authHeaders,
        cache: "no-store",
      },
    );
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        payload ?? { success: false, error: "failed to send test notification" },
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? { success: true });
  } catch (error) {
    return NextResponse.json(
      { success: false, error: error instanceof Error ? error.message : "backend error" },
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
