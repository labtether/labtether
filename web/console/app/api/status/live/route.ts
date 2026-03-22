import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

type ProxyTimingBreakdown = {
  totalMs: number;
  prepareMs?: number;
  upstreamFetchMs?: number;
  upstreamReadMs?: number;
};

function emptyLiveStatusResponse() {
  return {
    timestamp: new Date().toISOString(),
    summary: {
      servicesUp: 0,
      servicesTotal: 0,
      assetCount: 0,
      staleAssetCount: 0,
    },
    endpoints: [],
    assets: [],
    telemetryOverview: [],
  };
}

function withProxyTimingHeaders(
  headers: Record<string, string>,
  timing: ProxyTimingBreakdown,
): Record<string, string> {
  const next = { ...headers };
  next["X-Labtether-Proxy-Total-Ms"] = formatTimingValue(timing.totalMs);
  if (timing.prepareMs != null) {
    next["X-Labtether-Proxy-Prepare-Ms"] = formatTimingValue(timing.prepareMs);
  }
  if (timing.upstreamFetchMs != null) {
    next["X-Labtether-Upstream-Fetch-Ms"] = formatTimingValue(timing.upstreamFetchMs);
  }
  if (timing.upstreamReadMs != null) {
    next["X-Labtether-Upstream-Read-Ms"] = formatTimingValue(timing.upstreamReadMs);
  }
  return next;
}

function formatTimingValue(value: number): string {
  if (!Number.isFinite(value) || value < 0) {
    return "0.00";
  }
  return value.toFixed(2);
}

function nowMs(): number {
  return typeof performance !== "undefined" ? performance.now() : Date.now();
}

export async function GET(request: Request) {
  const startedAt = nowMs();
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const incoming = new URL(request.url);
  const groupFilter = incoming.searchParams.get("group_id")?.trim() ?? "";

  const statusURL = new URL(`${base.api}/status/aggregate/live`);
  statusURL.searchParams.set("caller", "console.status.live");
  if (groupFilter) {
    statusURL.searchParams.set("group_id", groupFilter);
  }
  const preparedAt = nowMs();

  try {
    const fetchStartedAt = nowMs();
    const backendResponse = await fetch(statusURL.toString(), {
      cache: "no-store",
      headers: authHeaders,
    });
    const upstreamFetchMs = nowMs() - fetchStartedAt;
    const readStartedAt = nowMs();
    if (!backendResponse.ok) {
      const payload = (await backendResponse.json().catch(() => null)) as { error?: string } | null;
      return NextResponse.json(payload ?? { error: "failed to load live status" }, {
        status: backendResponse.status,
        headers: withProxyTimingHeaders({
          "X-Labtether-Backend-Calls": "1",
        }, {
          totalMs: nowMs() - startedAt,
          prepareMs: preparedAt - startedAt,
          upstreamFetchMs,
          upstreamReadMs: nowMs() - readStartedAt,
        }),
      });
    }

    const payload = await backendResponse.json();
    return NextResponse.json(payload, {
      headers: withProxyTimingHeaders({
        "X-Labtether-Backend-Calls": "1",
      }, {
        totalMs: nowMs() - startedAt,
        prepareMs: preparedAt - startedAt,
        upstreamFetchMs,
        upstreamReadMs: nowMs() - readStartedAt,
      }),
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load live status" },
      {
        status: 502,
        headers: withProxyTimingHeaders({
          "X-Labtether-Backend-Calls": "1",
        }, {
          totalMs: nowMs() - startedAt,
          prepareMs: preparedAt - startedAt,
        }),
      }
    );
  }
}
