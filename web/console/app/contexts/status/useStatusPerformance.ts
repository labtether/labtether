"use client";

import { useEffect, useRef } from "react";
import { currentRoutePerfName, reportRoutePerfMetric } from "../../hooks/useRoutePerfTelemetry";
import type { StatusResponse } from "../../console/models";

type StatusRefreshSource = "live" | "full";

export type PendingStatusPerf = {
  route: ReturnType<typeof currentRoutePerfName>;
  source: StatusRefreshSource;
  startedAt: number;
  requestDurationMs: number;
  proxyTotalDurationMs: number;
  proxyPrepareDurationMs: number;
  upstreamFetchDurationMs: number;
  upstreamReadDurationMs: number;
  browserRequestOverheadMs: number;
  parseDurationMs: number;
  compareDurationMs: number;
  mergeDurationMs: number;
  postResponseToPaintDurationMs: number;
  uncategorizedDurationMs: number;
  longTaskCount: number;
  longTaskTotalDurationMs: number;
  longTaskMaxDurationMs: number;
  decision: "unchanged" | "synthesized" | "merged" | "not_modified";
  groupFiltered: boolean;
  assetCount: number;
  telemetryCount: number;
};

type AfterPaintTimings = {
  firstFrameAt: number;
  settledAt: number;
};

function afterPaint(callback: (timings: AfterPaintTimings) => void): void {
  if (typeof window === "undefined") {
    return;
  }
  const scheduleFrame = typeof window.requestAnimationFrame === "function"
    ? window.requestAnimationFrame.bind(window)
    : (fn: FrameRequestCallback) => window.setTimeout(() => fn(performance.now()), 0);
  scheduleFrame((firstFrameAt) => {
    scheduleFrame((settledAt) => {
      callback({ firstFrameAt, settledAt });
    });
  });
}

type ProxyTimingMetadata = {
  totalMs: number;
  prepareMs: number;
  upstreamFetchMs: number;
  upstreamReadMs: number;
};

type LongTaskSample = {
  startTime: number;
  duration: number;
};

type LongTaskWindowSummary = {
  count: number;
  totalDurationMs: number;
  maxDurationMs: number;
};

const longTaskBuffer: LongTaskSample[] = [];
let longTaskObserverStarted = false;

function ensureLongTaskObserver(): void {
  if (longTaskObserverStarted || typeof window === "undefined" || typeof PerformanceObserver !== "function") {
    return;
  }
  try {
    const observer = new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        if (entry.entryType !== "longtask") {
          continue;
        }
        longTaskBuffer.push({
          startTime: entry.startTime,
          duration: entry.duration,
        });
      }
      pruneLongTaskBuffer(performance.now());
    });
    observer.observe({ type: "longtask", buffered: true });
    longTaskObserverStarted = true;
  } catch {
    // Long task observation is best effort and unsupported in some browsers.
  }
}

function pruneLongTaskBuffer(nowMs: number): void {
  const oldestAllowed = nowMs - 60_000;
  let deleteCount = 0;
  while (deleteCount < longTaskBuffer.length) {
    const sample = longTaskBuffer[deleteCount];
    if ((sample.startTime + sample.duration) >= oldestAllowed) {
      break;
    }
    deleteCount += 1;
  }
  if (deleteCount > 0) {
    longTaskBuffer.splice(0, deleteCount);
  }
}

function summarizeLongTasks(startTimeMs: number, endTimeMs: number): LongTaskWindowSummary {
  if (typeof performance === "undefined" || endTimeMs <= startTimeMs) {
    return { count: 0, totalDurationMs: 0, maxDurationMs: 0 };
  }
  pruneLongTaskBuffer(performance.now());
  let count = 0;
  let totalDurationMs = 0;
  let maxDurationMs = 0;
  for (const sample of longTaskBuffer) {
    const sampleEnd = sample.startTime + sample.duration;
    if (sample.startTime >= endTimeMs || sampleEnd <= startTimeMs) {
      continue;
    }
    count += 1;
    totalDurationMs += sample.duration;
    maxDurationMs = Math.max(maxDurationMs, sample.duration);
  }
  return { count, totalDurationMs, maxDurationMs };
}

export function parseProxyTimingHeaders(headers: Headers): ProxyTimingMetadata {
  return {
    totalMs: parseTimingHeader(headers.get("x-labtether-proxy-total-ms")),
    prepareMs: parseTimingHeader(headers.get("x-labtether-proxy-prepare-ms")),
    upstreamFetchMs: parseTimingHeader(headers.get("x-labtether-upstream-fetch-ms")),
    upstreamReadMs: parseTimingHeader(headers.get("x-labtether-upstream-read-ms")),
  };
}

function parseTimingHeader(value: string | null): number {
  if (!value) {
    return 0;
  }
  const parsed = Number(value.trim());
  if (!Number.isFinite(parsed) || parsed < 0) {
    return 0;
  }
  return parsed;
}

export function useStatusPerformance(status: StatusResponse | null) {
  const pendingPerfRef = useRef<PendingStatusPerf | null>(null);
  useEffect(() => {
    ensureLongTaskObserver();
  }, []);

  useEffect(() => {
    const pending = pendingPerfRef.current;
    if (!pending || !status || !pending.route) {
      return;
    }
    pendingPerfRef.current = null;

    reportRoutePerfMetric({
      route: pending.route,
      metric: `status.${pending.source}.request`,
      durationMs: pending.requestDurationMs,
      sampleSize: pending.assetCount,
      metadata: {
        group_filtered: pending.groupFiltered,
        asset_count: pending.assetCount,
        telemetry_count: pending.telemetryCount,
      },
    });
    if (pending.proxyTotalDurationMs > 0) {
      reportRoutePerfMetric({
        route: pending.route,
        metric: `status.${pending.source}.proxy_total`,
        durationMs: pending.proxyTotalDurationMs,
        sampleSize: pending.assetCount,
        metadata: {
          group_filtered: pending.groupFiltered,
          decision: pending.decision,
        },
      });
    }
    if (pending.proxyPrepareDurationMs > 0) {
      reportRoutePerfMetric({
        route: pending.route,
        metric: `status.${pending.source}.proxy_prepare`,
        durationMs: pending.proxyPrepareDurationMs,
        sampleSize: pending.assetCount,
        metadata: {
          group_filtered: pending.groupFiltered,
        },
      });
    }
    if (pending.upstreamFetchDurationMs > 0) {
      reportRoutePerfMetric({
        route: pending.route,
        metric: `status.${pending.source}.proxy_fetch`,
        durationMs: pending.upstreamFetchDurationMs,
        sampleSize: pending.assetCount,
        metadata: {
          group_filtered: pending.groupFiltered,
        },
      });
    }
    if (pending.upstreamReadDurationMs > 0) {
      reportRoutePerfMetric({
        route: pending.route,
        metric: `status.${pending.source}.proxy_read`,
        durationMs: pending.upstreamReadDurationMs,
        sampleSize: pending.assetCount,
        metadata: {
          group_filtered: pending.groupFiltered,
        },
      });
    }
    if (pending.browserRequestOverheadMs > 0) {
      reportRoutePerfMetric({
        route: pending.route,
        metric: `status.${pending.source}.browser_overhead`,
        durationMs: pending.browserRequestOverheadMs,
        sampleSize: pending.assetCount,
        metadata: {
          group_filtered: pending.groupFiltered,
          decision: pending.decision,
        },
      });
    }
    reportRoutePerfMetric({
      route: pending.route,
      metric: `status.${pending.source}.parse`,
      durationMs: pending.parseDurationMs,
      sampleSize: pending.assetCount,
      metadata: {
        group_filtered: pending.groupFiltered,
      },
    });
    reportRoutePerfMetric({
      route: pending.route,
      metric: `status.${pending.source}.compare`,
      durationMs: pending.compareDurationMs,
      sampleSize: pending.assetCount,
      metadata: {
        decision: pending.decision,
        group_filtered: pending.groupFiltered,
      },
    });
    reportRoutePerfMetric({
      route: pending.route,
      metric: `status.${pending.source}.merge`,
      durationMs: pending.mergeDurationMs,
      sampleSize: pending.assetCount,
      metadata: {
        decision: pending.decision,
        group_filtered: pending.groupFiltered,
      },
    });
    const route = pending.route;
    afterPaint(({ firstFrameAt, settledAt }) => {
      const firstFrameDurationMs = firstFrameAt - pending.startedAt;
      const paintDurationMs = settledAt - pending.startedAt;
      const settleDelayDurationMs = Math.max(0, settledAt - firstFrameAt);
      const longTaskSummary = summarizeLongTasks(pending.startedAt, pending.startedAt + paintDurationMs);
      const postResponseToPaintDurationMs = Math.max(0, paintDurationMs - pending.requestDurationMs);
      const postResponseToFirstFrameDurationMs = Math.max(0, firstFrameDurationMs - pending.requestDurationMs);
      const uncategorizedDurationMs = Math.max(
        0,
        paintDurationMs
          - pending.requestDurationMs
          - pending.parseDurationMs
          - pending.compareDurationMs
          - pending.mergeDurationMs,
      );
      reportRoutePerfMetric({
        route,
        metric: `status.${pending.source}.first_frame`,
        durationMs: firstFrameDurationMs,
        sampleSize: pending.assetCount,
        metadata: {
          decision: pending.decision,
          group_filtered: pending.groupFiltered,
        },
      });
      reportRoutePerfMetric({
        route,
        metric: `status.${pending.source}.paint`,
        durationMs: paintDurationMs,
        sampleSize: pending.assetCount,
        metadata: {
          decision: pending.decision,
          group_filtered: pending.groupFiltered,
          telemetry_count: pending.telemetryCount,
          first_frame_ms: firstFrameDurationMs,
          settle_delay_ms: settleDelayDurationMs,
          post_response_to_first_frame_ms: postResponseToFirstFrameDurationMs,
          longtask_count: longTaskSummary.count,
          longtask_max_ms: longTaskSummary.maxDurationMs,
        },
      });
      reportRoutePerfMetric({
        route,
        metric: `status.${pending.source}.settle_delay`,
        durationMs: settleDelayDurationMs,
        sampleSize: pending.assetCount,
        metadata: {
          decision: pending.decision,
          group_filtered: pending.groupFiltered,
        },
      });
      reportRoutePerfMetric({
        route,
        metric: `status.${pending.source}.post_response_to_first_frame`,
        durationMs: postResponseToFirstFrameDurationMs,
        sampleSize: pending.assetCount,
        metadata: {
          decision: pending.decision,
          group_filtered: pending.groupFiltered,
        },
      });
      reportRoutePerfMetric({
        route,
        metric: `status.${pending.source}.post_response_to_paint`,
        durationMs: postResponseToPaintDurationMs,
        sampleSize: pending.assetCount,
        metadata: {
          decision: pending.decision,
          group_filtered: pending.groupFiltered,
        },
      });
      reportRoutePerfMetric({
        route,
        metric: `status.${pending.source}.uncategorized`,
        durationMs: uncategorizedDurationMs,
        sampleSize: pending.assetCount,
        metadata: {
          decision: pending.decision,
          group_filtered: pending.groupFiltered,
        },
      });
      if (longTaskSummary.totalDurationMs > 0) {
        reportRoutePerfMetric({
          route,
          metric: `status.${pending.source}.longtask_total`,
          durationMs: longTaskSummary.totalDurationMs,
          sampleSize: pending.assetCount,
          metadata: {
            decision: pending.decision,
            group_filtered: pending.groupFiltered,
            count: longTaskSummary.count,
          },
        });
        reportRoutePerfMetric({
          route,
          metric: `status.${pending.source}.longtask_max`,
          durationMs: longTaskSummary.maxDurationMs,
          sampleSize: pending.assetCount,
          metadata: {
            decision: pending.decision,
            group_filtered: pending.groupFiltered,
            count: longTaskSummary.count,
          },
        });
      }
    });
  }, [status]);

  return pendingPerfRef;
}
