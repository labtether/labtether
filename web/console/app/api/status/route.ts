import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../lib/backend";

export const dynamic = "force-dynamic";

type ProxyTimingBreakdown = {
  totalMs: number;
  prepareMs?: number;
  upstreamFetchMs?: number;
  upstreamReadMs?: number;
};

function emptyStatusResponse() {
  return {
    timestamp: new Date().toISOString(),
    summary: {
      servicesUp: 0,
      servicesTotal: 0,
      connectorCount: 0,
      groupCount: 0,
      assetCount: 0,
      sessionCount: 0,
      auditCount: 0,
      processedJobs: 0,
      actionRunCount: 0,
      updateRunCount: 0,
      deadLetterCount: 0,
      staleAssetCount: 0,
      retentionError: "",
    },
    endpoints: [],
    connectors: [],
    groups: [],
    assets: [],
    telemetryOverview: [],
    recentLogs: [],
    logSources: [],
    groupReliability: [],
    actionRuns: [],
    updatePlans: [],
    updateRuns: [],
    deadLetters: [],
    deadLetterAnalytics: {
      window: "24h",
      bucket: "1h",
      total: 0,
      rate_per_hour: 0,
      rate_per_day: 0,
      trend: [],
      top_components: [],
      top_subjects: [],
      top_error_classes: [],
    },
    sessions: [],
    recentCommands: [],
    recentAudit: [],
    canonical: {
      registry: {
        capabilities: [],
        operations: [],
        metrics: [],
        events: [],
        templates: [],
      },
      providers: [],
      capabilitySets: [],
      templateBindings: {},
      reconciliation: [],
    },
  };
}

function fullResponseHeadersFromBackend(response: Response): Record<string, string> {
  const headers: Record<string, string> = {
    "X-Labtether-Backend-Calls": "1",
    "Cache-Control": response.headers.get("cache-control") ?? "private, max-age=30",
  };
  const etag = response.headers.get("etag");
  if (etag) {
    headers["ETag"] = etag;
  }
  return headers;
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

  const statusURL = new URL(`${base.api}/status/aggregate`);
  statusURL.searchParams.set("caller", "console.status.full");
  if (groupFilter) {
    statusURL.searchParams.set("group_id", groupFilter);
  }

  const headers = new Headers(authHeaders);
  const ifNoneMatch = request.headers.get("if-none-match");
  if (ifNoneMatch) {
    headers.set("If-None-Match", ifNoneMatch);
  }
  const preparedAt = nowMs();

  try {
    const fetchStartedAt = nowMs();
    const backendResponse = await fetch(statusURL.toString(), {
      cache: "no-store",
      headers,
    });
    const upstreamFetchMs = nowMs() - fetchStartedAt;

    if (backendResponse.status === 304) {
      return new Response(null, {
        status: 304,
        headers: withProxyTimingHeaders(fullResponseHeadersFromBackend(backendResponse), {
          totalMs: nowMs() - startedAt,
          prepareMs: preparedAt - startedAt,
          upstreamFetchMs,
        }),
      });
    }

    const readStartedAt = nowMs();
    if (!backendResponse.ok) {
      const payload = (await backendResponse.json().catch(() => null)) as { error?: string } | null;
      return NextResponse.json(payload ?? { error: "failed to load status" }, {
        status: backendResponse.status,
        headers: withProxyTimingHeaders({
          "X-Labtether-Backend-Calls": "1",
          "Cache-Control": "private, max-age=30",
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
      headers: withProxyTimingHeaders(fullResponseHeadersFromBackend(backendResponse), {
        totalMs: nowMs() - startedAt,
        prepareMs: preparedAt - startedAt,
        upstreamFetchMs,
        upstreamReadMs: nowMs() - readStartedAt,
      }),
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load status" },
      {
        status: 502,
        headers: withProxyTimingHeaders({
          "X-Labtether-Backend-Calls": "1",
          "Cache-Control": "private, max-age=30",
        }, {
          totalMs: nowMs() - startedAt,
          prepareMs: preparedAt - startedAt,
        }),
      }
    );
  }
}
