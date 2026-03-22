"use client";

import { Profiler, useCallback, useEffect, useRef, type ProfilerOnRenderCallback, type ReactNode } from "react";
import type { RoutePerfName } from "../hooks/useRoutePerfTelemetry";
import { reportRoutePerfMetric } from "../hooks/useRoutePerfTelemetry";

type RoutePerfBoundaryProps = {
  route: RoutePerfName;
  sampleSize?: number;
  metadata?: Record<string, string | number | boolean | null | undefined>;
  children: ReactNode;
};

type PendingRouteRenderSummary = {
  commits: number;
  mountCommits: number;
  updateCommits: number;
  totalActualDurationMs: number;
  maxActualDurationMs: number;
  totalBaseDurationMs: number;
  maxBaseDurationMs: number;
  totalElapsedDurationMs: number;
  maxElapsedDurationMs: number;
};

const flushWindowMs = 5_000;

function createPendingRouteRenderSummary(): PendingRouteRenderSummary {
  return {
    commits: 0,
    mountCommits: 0,
    updateCommits: 0,
    totalActualDurationMs: 0,
    maxActualDurationMs: 0,
    totalBaseDurationMs: 0,
    maxBaseDurationMs: 0,
    totalElapsedDurationMs: 0,
    maxElapsedDurationMs: 0,
  };
}

export function RoutePerfBoundary({
  route,
  sampleSize = 0,
  metadata,
  children,
}: RoutePerfBoundaryProps) {
  const summaryRef = useRef<PendingRouteRenderSummary>(createPendingRouteRenderSummary());
  const metadataRef = useRef(metadata);
  const sampleSizeRef = useRef(sampleSize);
  const flushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  metadataRef.current = metadata;
  sampleSizeRef.current = sampleSize;

  const flushMetrics = useCallback(() => {
    flushTimerRef.current = null;
    const pending = summaryRef.current;
    if (pending.commits === 0) {
      return;
    }

    const commits = pending.commits;
    const baseMetadata = {
      ...metadataRef.current,
      commit_count: commits,
      mount_commits: pending.mountCommits,
      update_commits: pending.updateCommits,
      max_actual_duration_ms: roundMetric(pending.maxActualDurationMs),
      max_base_duration_ms: roundMetric(pending.maxBaseDurationMs),
      max_elapsed_duration_ms: roundMetric(pending.maxElapsedDurationMs),
    };

    reportRoutePerfMetric({
      route,
      metric: "render.route_commit.actual_avg",
      durationMs: pending.totalActualDurationMs / commits,
      sampleSize: sampleSizeRef.current,
      metadata: baseMetadata,
      throttleMs: 0,
    });
    reportRoutePerfMetric({
      route,
      metric: "render.route_commit.base_avg",
      durationMs: pending.totalBaseDurationMs / commits,
      sampleSize: sampleSizeRef.current,
      metadata: baseMetadata,
      throttleMs: 0,
    });
    reportRoutePerfMetric({
      route,
      metric: "render.route_commit.elapsed_avg",
      durationMs: pending.totalElapsedDurationMs / commits,
      sampleSize: sampleSizeRef.current,
      metadata: baseMetadata,
      throttleMs: 0,
    });

    summaryRef.current = createPendingRouteRenderSummary();
  }, [route]);

  useEffect(() => () => {
    if (flushTimerRef.current) {
      clearTimeout(flushTimerRef.current);
    }
    flushMetrics();
  }, [flushMetrics]);

  const onRender = useCallback<ProfilerOnRenderCallback>((_id, phase, actualDuration, baseDuration, startTime, commitTime) => {
    const summary = summaryRef.current;
    summary.commits += 1;
    summary.mountCommits += phase === "mount" ? 1 : 0;
    summary.updateCommits += phase === "update" ? 1 : 0;
    summary.totalActualDurationMs += actualDuration;
    summary.maxActualDurationMs = Math.max(summary.maxActualDurationMs, actualDuration);
    summary.totalBaseDurationMs += baseDuration;
    summary.maxBaseDurationMs = Math.max(summary.maxBaseDurationMs, baseDuration);
    const elapsedDurationMs = commitTime - startTime;
    summary.totalElapsedDurationMs += elapsedDurationMs;
    summary.maxElapsedDurationMs = Math.max(summary.maxElapsedDurationMs, elapsedDurationMs);

    if (flushTimerRef.current != null) {
      return;
    }
    flushTimerRef.current = setTimeout(() => {
      flushMetrics();
    }, flushWindowMs);
  }, [flushMetrics]);

  return (
    <Profiler id={`route:${route}`} onRender={onRender}>
      {children}
    </Profiler>
  );
}

function roundMetric(value: number): number {
  return Math.round(value * 100) / 100;
}
