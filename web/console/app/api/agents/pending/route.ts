import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, backendBaseURLs } from "../../../../lib/backend";

export interface PendingAgent {
  asset_id: string;
  hostname: string;
  platform: string;
  remote_ip: string;
  connected_at: string;
  device_fingerprint?: string;
  device_key_alg?: string;
  identity_verified: boolean;
  identity_verified_at?: string;
}

export async function GET(request: Request) {
  try {
    const base = backendBaseURLs();
    const response = await fetch(`${base.api}/api/v1/agents/pending`, {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });

    if (!response.ok) {
      const payload = (await response.json().catch(() => null)) as { error?: string } | null;
      return NextResponse.json(payload ?? { error: "failed to load pending agents" }, { status: response.status });
    }

    const data = (await response.json()) as { count?: number; agents?: PendingAgent[] };
    return NextResponse.json({ count: data.count ?? 0, agents: data.agents ?? [] });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load pending agents" },
      { status: 502 }
    );
  }
}
