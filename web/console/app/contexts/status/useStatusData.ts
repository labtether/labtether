"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { StatusResponse } from "../../console/models";
import { currentRoutePerfName, reportRoutePerfMetric } from "../../hooks/useRoutePerfTelemetry";
import { mergeFullStatus, mergeLiveStatus, normalizeLiveStatusResponse, normalizeStatusResponse } from "./statusPayload";
import { parseProxyTimingHeaders, useStatusPerformance } from "./useStatusPerformance";

export function useStatusData(selectedGroupFilter: string) {
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const currentStatusRef = useRef<StatusResponse | null>(null);
  const pendingPerfRef = useStatusPerformance(status);
  // Holds the last successfully fetched full status payload for merging
  const fullStatusRef = useRef<StatusResponse | null>(null);
  // Holds the ETag from the last full status response for 304 optimization
  const fullStatusETagRef = useRef<string | null>(null);
  // Monotonic scope generation to invalidate stale async responses when the group filter changes.
  const statusScopeRef = useRef(0);
  const liveRequestSeqRef = useRef(0);
  const fullRequestSeqRef = useRef(0);
  const liveFetchInFlightRef = useRef(false);
  const fullFetchInFlightRef = useRef(false);

  // Build group-filtered query string helper
  const buildGroupQuery = useCallback(
    (groupFilter: string): string => {
      const params = new URLSearchParams();
      if (groupFilter !== "all") {
        params.set("group_id", groupFilter);
      }
      const qs = params.toString();
      return qs ? `?${qs}` : "";
    },
    []
  );

  // Fast fetch: live endpoint — updates assets, telemetry, summary, endpoints
  const fetchLiveStatus = useCallback(async (groupFilter: string) => {
    if (liveFetchInFlightRef.current) {
      return;
    }
    liveFetchInFlightRef.current = true;
    const scopeID = statusScopeRef.current;
    const requestID = ++liveRequestSeqRef.current;
    const route = currentRoutePerfName();
    const startedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
    try {
      const query = buildGroupQuery(groupFilter);
      const response = await fetch(`/api/status/live${query}`, { cache: "no-store" });
      if (!response.ok) {
        throw new Error(`live status fetch failed: ${response.status}`);
      }

      const requestDurationMs = (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt;
      const proxyTiming = parseProxyTimingHeaders(response.headers);
      const parseStartedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
      const live = normalizeLiveStatusResponse(await response.json().catch(() => null));
      const parseDurationMs = (typeof performance !== "undefined" ? performance.now() : Date.now()) - parseStartedAt;
      if (scopeID !== statusScopeRef.current || requestID !== liveRequestSeqRef.current) {
        return;
      }

      const compareStartedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
      const current = currentStatusRef.current;
      const { status: nextStatus, decision } = mergeLiveStatus(current, fullStatusRef.current, live);
      const compareDurationMs = (typeof performance !== "undefined" ? performance.now() : Date.now()) - compareStartedAt;

      const mergeStartedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
      if (nextStatus && nextStatus !== current) {
        currentStatusRef.current = nextStatus;
        setStatus(nextStatus);
      }
      const mergeDurationMs = (typeof performance !== "undefined" ? performance.now() : Date.now()) - mergeStartedAt;

      if (decision !== "unchanged") {
        pendingPerfRef.current = {
          route,
          source: "live",
          startedAt,
          requestDurationMs,
          proxyTotalDurationMs: proxyTiming.totalMs,
          proxyPrepareDurationMs: proxyTiming.prepareMs,
          upstreamFetchDurationMs: proxyTiming.upstreamFetchMs,
          upstreamReadDurationMs: proxyTiming.upstreamReadMs,
          browserRequestOverheadMs: Math.max(0, requestDurationMs - proxyTiming.totalMs),
          parseDurationMs,
          compareDurationMs,
          mergeDurationMs,
          postResponseToPaintDurationMs: 0,
          uncategorizedDurationMs: 0,
          longTaskCount: 0,
          longTaskTotalDurationMs: 0,
          longTaskMaxDurationMs: 0,
          decision,
          groupFiltered: groupFilter !== "all",
          assetCount: live.assets.length,
          telemetryCount: live.telemetryOverview.length,
        };
      } else if (route) {
        reportRoutePerfMetric({
          route,
          metric: "status.live.compare",
          durationMs: compareDurationMs,
          sampleSize: live.assets.length,
          metadata: {
            decision,
            group_filtered: groupFilter !== "all",
          },
        });
      }

      setError(null);
    } catch (err) {
      if (scopeID !== statusScopeRef.current || requestID !== liveRequestSeqRef.current) {
        return;
      }
      if (route) {
        reportRoutePerfMetric({
          route,
          metric: "status.live.request",
          durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
          status: "error",
          metadata: {
            group_filtered: groupFilter !== "all",
          },
        });
      }
      setError(err instanceof Error ? err.message : "live status unavailable");
    } finally {
      if (scopeID === statusScopeRef.current && requestID === liveRequestSeqRef.current) {
        liveFetchInFlightRef.current = false;
        setLoading(false);
      }
    }
  }, [buildGroupQuery, pendingPerfRef]);

  // Slow fetch: full endpoint — updates everything else (sessions, audit, actionRuns, etc.)
  const fetchFullStatus = useCallback(async (groupFilter: string) => {
    if (fullFetchInFlightRef.current) {
      return;
    }
    fullFetchInFlightRef.current = true;
    const scopeID = statusScopeRef.current;
    const requestID = ++fullRequestSeqRef.current;
    const route = currentRoutePerfName();
    const startedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
    try {
      const query = buildGroupQuery(groupFilter);

      const response = await fetch(`/api/status${query}`, {
        cache: "no-store",
        headers: fullStatusETagRef.current
          ? { "If-None-Match": fullStatusETagRef.current }
          : undefined,
      });
      const requestDurationMs = (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt;
      const proxyTiming = parseProxyTimingHeaders(response.headers);

      if (scopeID !== statusScopeRef.current || requestID !== fullRequestSeqRef.current) {
        return;
      }

      // 304 Not Modified — nothing changed, keep existing state
      if (response.status === 304) {
        if (route) {
          reportRoutePerfMetric({
            route,
            metric: "status.full.request",
            durationMs: requestDurationMs,
            sampleSize: currentStatusRef.current?.assets.length ?? 0,
            metadata: {
              decision: "not_modified",
              group_filtered: groupFilter !== "all",
              proxy_total_ms: proxyTiming.totalMs,
              browser_overhead_ms: Math.max(0, requestDurationMs - proxyTiming.totalMs),
            },
          });
          if (proxyTiming.totalMs > 0) {
            reportRoutePerfMetric({
              route,
              metric: "status.full.proxy_total",
              durationMs: proxyTiming.totalMs,
              sampleSize: currentStatusRef.current?.assets.length ?? 0,
              metadata: {
                decision: "not_modified",
                group_filtered: groupFilter !== "all",
              },
            });
          }
        }
        return;
      }

      if (!response.ok) {
        throw new Error(`status fetch failed: ${response.status}`);
      }

      const parseStartedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
      const payload = normalizeStatusResponse(await response.json().catch(() => null));
      const parseDurationMs = (typeof performance !== "undefined" ? performance.now() : Date.now()) - parseStartedAt;
      // Parsing can yield while the selected group changes. Only the current
      // request may publish its payload or validator into that group's cache.
      if (scopeID !== statusScopeRef.current || requestID !== fullRequestSeqRef.current) {
        return;
      }
      fullStatusETagRef.current = response.headers.get("etag");
      fullStatusRef.current = payload;

      const compareStartedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
      const current = currentStatusRef.current;
      const { status: nextStatus, decision } = mergeFullStatus(current, payload);
      const compareDurationMs = (typeof performance !== "undefined" ? performance.now() : Date.now()) - compareStartedAt;

      const mergeStartedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
      if (nextStatus !== current) {
        currentStatusRef.current = nextStatus;
        setStatus(nextStatus);
      }
      const mergeDurationMs = (typeof performance !== "undefined" ? performance.now() : Date.now()) - mergeStartedAt;

      if (decision !== "unchanged") {
        pendingPerfRef.current = {
          route,
          source: "full",
          startedAt,
          requestDurationMs,
          proxyTotalDurationMs: proxyTiming.totalMs,
          proxyPrepareDurationMs: proxyTiming.prepareMs,
          upstreamFetchDurationMs: proxyTiming.upstreamFetchMs,
          upstreamReadDurationMs: proxyTiming.upstreamReadMs,
          browserRequestOverheadMs: Math.max(0, requestDurationMs - proxyTiming.totalMs),
          parseDurationMs,
          compareDurationMs,
          mergeDurationMs,
          postResponseToPaintDurationMs: 0,
          uncategorizedDurationMs: 0,
          longTaskCount: 0,
          longTaskTotalDurationMs: 0,
          longTaskMaxDurationMs: 0,
          decision,
          groupFiltered: groupFilter !== "all",
          assetCount: nextStatus.assets.length,
          telemetryCount: nextStatus.telemetryOverview.length,
        };
      } else if (route) {
        reportRoutePerfMetric({
          route,
          metric: "status.full.compare",
          durationMs: compareDurationMs,
          sampleSize: nextStatus.assets.length,
          metadata: {
            decision,
            group_filtered: groupFilter !== "all",
          },
        });
      }

      setError(null);
    } catch (err) {
      if (scopeID !== statusScopeRef.current || requestID !== fullRequestSeqRef.current) {
        return;
      }
      if (route) {
        reportRoutePerfMetric({
          route,
          metric: "status.full.request",
          durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
          status: "error",
          metadata: {
            group_filtered: groupFilter !== "all",
          },
        });
      }
      setError(err instanceof Error ? err.message : "status unavailable");
    } finally {
      if (scopeID === statusScopeRef.current && requestID === fullRequestSeqRef.current) {
        fullFetchInFlightRef.current = false;
        setLoading(false);
      }
    }
  }, [buildGroupQuery, pendingPerfRef]);

  // fetchStatus is the public API exposed on the context — triggers both polls
  const fetchStatus = useCallback(async () => {
    await Promise.all([
      fetchLiveStatus(selectedGroupFilter),
      fetchFullStatus(selectedGroupFilter),
    ]);
  }, [fetchLiveStatus, fetchFullStatus, selectedGroupFilter]);

  useEffect(() => {
    statusScopeRef.current += 1;
    liveFetchInFlightRef.current = false;
    fullFetchInFlightRef.current = false;
    fullStatusRef.current = null;
    fullStatusETagRef.current = null;
    currentStatusRef.current = null;
    pendingPerfRef.current = null;
    return () => {
      // Retired requests cannot block a new group or publish after unmount.
      statusScopeRef.current += 1;
      liveFetchInFlightRef.current = false;
      fullFetchInFlightRef.current = false;
    };
  }, [selectedGroupFilter, pendingPerfRef]);

  return { status, loading, error, fetchStatus, fetchLiveStatus, fetchFullStatus };
}
