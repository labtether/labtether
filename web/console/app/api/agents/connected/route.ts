import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, backendBaseURLs } from "../../../../lib/backend";

export async function GET(request: Request) {
  try {
    const base = backendBaseURLs();
    const response = await fetch(`${base.api}/agents/connected`, {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });

    if (!response.ok) {
      const payload = (await response.json().catch(() => null)) as { error?: string } | null;
      return NextResponse.json(
        payload ?? { error: "failed to load connected agents" },
        { status: response.status },
      );
    }

    const data = (await response.json()) as {
      count?: number;
      assets?: string[];
      assetsInfo?: Array<{ id: string; has_tmux: boolean; platform?: string }>;
    };
    return NextResponse.json({
      count: data.count ?? 0,
      assets: data.assets ?? [],
      assetsInfo: data.assetsInfo ?? [],
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load connected agents" },
      { status: 502 },
    );
  }
}
