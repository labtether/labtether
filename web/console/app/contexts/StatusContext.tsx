"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  parseBoolSetting,
  parseIntSetting,
  parseWindowSetting,
  settingValue
} from "../console/formatters";
import { buildBrowserWsUrl } from "../lib/ws";
import { currentRoutePerfName, reportRoutePerfMetric } from "../hooks/useRoutePerfTelemetry";
import { runtimeSettingKeys, telemetryWindows } from "../console/models";
import type {
  LiveStatusResponse,
  RuntimeSettingEntry,
  RuntimeSettingsPayload,
  StatusResponse,
  TelemetryWindow
} from "../console/models";

type StatusContextValue = {
  status: StatusResponse | null;
  loading: boolean;
  error: string | null;
  selectedGroupFilter: string;
  setSelectedGroupFilter: (value: string) => void;
  fetchStatus: () => Promise<void>;
  runtimeSettings: RuntimeSettingEntry[];
  pollIntervalSeconds: number;
  defaultTelemetryWindow: TelemetryWindow;
  defaultLogWindow: TelemetryWindow;
  logQueryLimit: number;
  defaultActorID: string;
  defaultActionDryRun: boolean;
  defaultUpdateDryRun: boolean;
  serviceStatusLabel: string;
  groupLabelByID: Map<string, string>;
};

type StatusControlsValue = {
  loading: boolean;
  error: string | null;
  selectedGroupFilter: string;
  setSelectedGroupFilter: (value: string) => void;
  fetchStatus: () => Promise<void>;
};

type StatusSettingsValue = {
  runtimeSettings: RuntimeSettingEntry[];
  pollIntervalSeconds: number;
  defaultTelemetryWindow: TelemetryWindow;
  defaultLogWindow: TelemetryWindow;
  logQueryLimit: number;
  defaultActorID: string;
  defaultActionDryRun: boolean;
  defaultUpdateDryRun: boolean;
};

type FastStatusSummary = Pick<StatusResponse["summary"], "servicesUp" | "servicesTotal" | "assetCount" | "staleAssetCount">;
type SlowStatusSummary = Pick<
  StatusResponse["summary"],
  | "connectorCount"
  | "groupCount"
  | "sessionCount"
  | "auditCount"
  | "processedJobs"
  | "actionRunCount"
  | "updateRunCount"
  | "deadLetterCount"
  | "retentionError"
>;

export type FastStatusSlice = Pick<StatusResponse, "timestamp" | "endpoints" | "assets" | "telemetryOverview"> & {
  summary: FastStatusSummary;
};

export type SlowStatusSlice = Pick<
  StatusResponse,
  | "connectors"
  | "groups"
  | "recentLogs"
  | "logSources"
  | "groupReliability"
  | "actionRuns"
  | "updatePlans"
  | "updateRuns"
  | "deadLetters"
  | "deadLetterAnalytics"
  | "sessions"
  | "recentCommands"
  | "recentAudit"
  | "canonical"
> & {
  summary: SlowStatusSummary;
};

type StatusFastValue = {
  status: FastStatusSlice | null;
  serviceStatusLabel: string;
};

type StatusSlowValue = {
  status: SlowStatusSlice | null;
  groupLabelByID: Map<string, string>;
};

const StatusContext = createContext<StatusContextValue | null>(null);
const StatusControlsContext = createContext<StatusControlsValue | null>(null);
const StatusSettingsContext = createContext<StatusSettingsValue | null>(null);
const StatusFastContext = createContext<StatusFastValue | null>(null);
const StatusSlowContext = createContext<StatusSlowValue | null>(null);
const StatusAssetNameMapContext = createContext<Map<string, string> | null>(null);

// Slow poll interval: full status endpoint (includes heavier fields such as
// log source aggregation). Keep it intentionally slower than live polling.
const SLOW_POLL_MS = 120_000;

type StatusRefreshSource = "live" | "full";

type PendingStatusPerf = {
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

function parseProxyTimingHeaders(headers: Headers): ProxyTimingMetadata {
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
  const parsed = Number.parseFloat(value);
  if (!Number.isFinite(parsed) || parsed < 0) {
    return 0;
  }
  return parsed;
}

function areStringMapsEqual(left: Map<string, string>, right: Map<string, string>): boolean {
  if (left === right) {
    return true;
  }
  if (left.size !== right.size) {
    return false;
  }
  for (const [key, value] of left) {
    if (right.get(key) !== value) {
      return false;
    }
  }
  return true;
}

function asObject(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function asString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function asNumber(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function ensureArray<T>(value: unknown): T[] {
  return Array.isArray(value) ? value as T[] : [];
}

function normalizeDeadLetterAnalytics(value: unknown): StatusResponse["deadLetterAnalytics"] {
  const raw = asObject(value) ?? {};
  return {
    window: asString(raw.window) || "24h",
    bucket: asString(raw.bucket) || "1h",
    total: asNumber(raw.total),
    rate_per_hour: asNumber(raw.rate_per_hour),
    rate_per_day: asNumber(raw.rate_per_day),
    trend: ensureArray<StatusResponse["deadLetterAnalytics"]["trend"][number]>(raw.trend),
    top_components: ensureArray<StatusResponse["deadLetterAnalytics"]["top_components"][number]>(raw.top_components),
    top_subjects: ensureArray<StatusResponse["deadLetterAnalytics"]["top_subjects"][number]>(raw.top_subjects),
    top_error_classes: ensureArray<StatusResponse["deadLetterAnalytics"]["top_error_classes"][number]>(raw.top_error_classes),
  };
}

function normalizeLiveStatusResponse(value: unknown): LiveStatusResponse {
  const raw = asObject(value) ?? {};
  const summary = asObject(raw.summary) ?? {};
  return {
    timestamp: asString(raw.timestamp),
    summary: {
      servicesUp: asNumber(summary.servicesUp),
      servicesTotal: asNumber(summary.servicesTotal),
      assetCount: asNumber(summary.assetCount),
      staleAssetCount: asNumber(summary.staleAssetCount),
    },
    endpoints: ensureArray<LiveStatusResponse["endpoints"][number]>(raw.endpoints),
    assets: ensureArray<LiveStatusResponse["assets"][number]>(raw.assets),
    telemetryOverview: ensureArray<LiveStatusResponse["telemetryOverview"][number]>(raw.telemetryOverview),
  };
}

function normalizeStatusResponse(value: unknown): StatusResponse {
  const raw = asObject(value) ?? {};
  const summary = asObject(raw.summary) ?? {};
  return {
    timestamp: asString(raw.timestamp),
    summary: {
      servicesUp: asNumber(summary.servicesUp),
      servicesTotal: asNumber(summary.servicesTotal),
      connectorCount: asNumber(summary.connectorCount),
      groupCount: asNumber(summary.groupCount),
      assetCount: asNumber(summary.assetCount),
      sessionCount: asNumber(summary.sessionCount),
      auditCount: asNumber(summary.auditCount),
      processedJobs: asNumber(summary.processedJobs),
      actionRunCount: asNumber(summary.actionRunCount),
      updateRunCount: asNumber(summary.updateRunCount),
      deadLetterCount: asNumber(summary.deadLetterCount),
      staleAssetCount: asNumber(summary.staleAssetCount),
      retentionError: asString(summary.retentionError) || undefined,
    },
    endpoints: ensureArray<StatusResponse["endpoints"][number]>(raw.endpoints),
    connectors: ensureArray<StatusResponse["connectors"][number]>(raw.connectors),
    groups: ensureArray<StatusResponse["groups"][number]>(raw.groups),
    assets: ensureArray<StatusResponse["assets"][number]>(raw.assets),
    telemetryOverview: ensureArray<StatusResponse["telemetryOverview"][number]>(raw.telemetryOverview),
    recentLogs: ensureArray<StatusResponse["recentLogs"][number]>(raw.recentLogs),
    logSources: ensureArray<StatusResponse["logSources"][number]>(raw.logSources),
    groupReliability: ensureArray<StatusResponse["groupReliability"][number]>(raw.groupReliability),
    actionRuns: ensureArray<StatusResponse["actionRuns"][number]>(raw.actionRuns),
    updatePlans: ensureArray<StatusResponse["updatePlans"][number]>(raw.updatePlans),
    updateRuns: ensureArray<StatusResponse["updateRuns"][number]>(raw.updateRuns),
    deadLetters: ensureArray<StatusResponse["deadLetters"][number]>(raw.deadLetters),
    deadLetterAnalytics: normalizeDeadLetterAnalytics(raw.deadLetterAnalytics),
    sessions: ensureArray<StatusResponse["sessions"][number]>(raw.sessions),
    recentCommands: ensureArray<StatusResponse["recentCommands"][number]>(raw.recentCommands),
    recentAudit: ensureArray<StatusResponse["recentAudit"][number]>(raw.recentAudit),
    canonical: asObject(raw.canonical) as StatusResponse["canonical"] | undefined,
  };
}

function areUnknownValuesEqual(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) {
    return true;
  }

  if (Array.isArray(left) || Array.isArray(right)) {
    if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) {
      return false;
    }
    for (let index = 0; index < left.length; index += 1) {
      if (!areUnknownValuesEqual(left[index], right[index])) {
        return false;
      }
    }
    return true;
  }

  if (left && right && typeof left === "object" && typeof right === "object") {
    const leftObj = left as Record<string, unknown>;
    const rightObj = right as Record<string, unknown>;
    const leftKeys = Object.keys(leftObj);
    const rightKeys = Object.keys(rightObj);
    if (leftKeys.length !== rightKeys.length) {
      return false;
    }
    for (const key of leftKeys) {
      if (!(key in rightObj)) {
        return false;
      }
      if (!areUnknownValuesEqual(leftObj[key], rightObj[key])) {
        return false;
      }
    }
    return true;
  }

  return false;
}

function areLiveSummaryEqual(
  left: StatusResponse["summary"] | LiveStatusResponse["summary"],
  right: LiveStatusResponse["summary"],
): boolean {
  return (
    left.servicesUp === right.servicesUp
    && left.servicesTotal === right.servicesTotal
    && left.assetCount === right.assetCount
    && left.staleAssetCount === right.staleAssetCount
  );
}

// Compare assets by fields that affect UI decisions, ignoring volatile
// timestamps (last_seen_at changes on every heartbeat but only matters when
// an asset goes stale — the staleAssetCount summary field covers that).
function areLiveAssetsEqual(
  left: LiveStatusResponse["assets"],
  right: LiveStatusResponse["assets"],
): boolean {
  if (left.length !== right.length) return false;
  for (let i = 0; i < left.length; i += 1) {
    const a = left[i];
    const b = right[i];
    if (
      a.id !== b.id
      || a.name !== b.name
      || a.status !== b.status
      || a.type !== b.type
      || a.source !== b.source
      || a.group_id !== b.group_id
      || a.platform !== b.platform
      || a.resource_class !== b.resource_class
      || a.resource_kind !== b.resource_kind
    ) {
      return false;
    }
  }
  return true;
}

// Compare telemetry by identity and rounded metrics so that sub-percent
// fluctuations between poll cycles don't trigger full UI re-renders.
function areLiveTelemetryEqual(
  left: LiveStatusResponse["telemetryOverview"],
  right: LiveStatusResponse["telemetryOverview"],
): boolean {
  if (left.length !== right.length) return false;
  for (let i = 0; i < left.length; i += 1) {
    const a = left[i];
    const b = right[i];
    if (
      a.asset_id !== b.asset_id
      || a.status !== b.status
      || Math.round(a.metrics.cpu_used_percent ?? 0) !== Math.round(b.metrics.cpu_used_percent ?? 0)
      || Math.round(a.metrics.memory_used_percent ?? 0) !== Math.round(b.metrics.memory_used_percent ?? 0)
      || Math.round(a.metrics.disk_used_percent ?? 0) !== Math.round(b.metrics.disk_used_percent ?? 0)
      || Math.round(a.metrics.temperature_celsius ?? 0) !== Math.round(b.metrics.temperature_celsius ?? 0)
    ) {
      return false;
    }
  }
  return true;
}

function areLiveSlicesEqual(current: StatusResponse, live: LiveStatusResponse): boolean {
  return (
    areLiveSummaryEqual(current.summary, live.summary)
    && areUnknownValuesEqual(current.endpoints, live.endpoints)
    && areLiveAssetsEqual(current.assets, live.assets)
    && areLiveTelemetryEqual(current.telemetryOverview, live.telemetryOverview)
  );
}

function areSlowSummaryEqual(
  left: StatusResponse["summary"],
  right: StatusResponse["summary"],
): boolean {
  return (
    left.connectorCount === right.connectorCount
    && left.groupCount === right.groupCount
    && left.sessionCount === right.sessionCount
    && left.auditCount === right.auditCount
    && left.processedJobs === right.processedJobs
    && left.actionRunCount === right.actionRunCount
    && left.updateRunCount === right.updateRunCount
    && left.deadLetterCount === right.deadLetterCount
    && left.retentionError === right.retentionError
  );
}

function areSlowStatusSlicesEqual(left: StatusResponse, right: StatusResponse): boolean {
  return (
    areSlowSummaryEqual(left.summary, right.summary)
    && areUnknownValuesEqual(left.connectors, right.connectors)
    && areUnknownValuesEqual(left.groups, right.groups)
    && areUnknownValuesEqual(left.recentLogs, right.recentLogs)
    && areUnknownValuesEqual(left.logSources, right.logSources)
    && areUnknownValuesEqual(left.groupReliability, right.groupReliability)
    && areUnknownValuesEqual(left.actionRuns, right.actionRuns)
    && areUnknownValuesEqual(left.updatePlans, right.updatePlans)
    && areUnknownValuesEqual(left.updateRuns, right.updateRuns)
    && areUnknownValuesEqual(left.deadLetters, right.deadLetters)
    && areUnknownValuesEqual(left.deadLetterAnalytics, right.deadLetterAnalytics)
    && areUnknownValuesEqual(left.sessions, right.sessions)
    && areUnknownValuesEqual(left.recentCommands, right.recentCommands)
    && areUnknownValuesEqual(left.recentAudit, right.recentAudit)
    && areUnknownValuesEqual(left.canonical, right.canonical)
  );
}

export function StatusProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedGroupFilter, setSelectedGroupFilter] = useState<string>("all");
  const [runtimeSettings, setRuntimeSettings] = useState<RuntimeSettingEntry[]>([]);
  const assetNameMapRef = useRef<Map<string, string>>(new Map());
  const currentStatusRef = useRef<StatusResponse | null>(null);
  const pendingPerfRef = useRef<PendingStatusPerf | null>(null);

  // Holds the last successfully fetched full status payload for merging
  const fullStatusRef = useRef<StatusResponse | null>(null);
  // Holds the ETag from the last full status response for 304 optimization
  const fullStatusETagRef = useRef<string | null>(null);
  // Monotonic scope generation to invalidate stale async responses when the group filter changes.
  const statusScopeRef = useRef(0);
  const previousGroupFilterRef = useRef(selectedGroupFilter);
  const liveRequestSeqRef = useRef(0);
  const fullRequestSeqRef = useRef(0);
  const liveFetchInFlightRef = useRef(false);
  const fullFetchInFlightRef = useRef(false);

  useEffect(() => {
    currentStatusRef.current = status;
  }, [status]);

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

  // Keep a ref so the WebSocket effect can read the current value without
  // being in its dependency array (changing group filter must NOT reconnect WS).
  const groupFilterRef = useRef(selectedGroupFilter);
  useEffect(() => {
    groupFilterRef.current = selectedGroupFilter;
  }, [selectedGroupFilter]);

  const pollIntervalSeconds = useMemo(
    () => parseIntSetting(settingValue(runtimeSettings, runtimeSettingKeys.pollIntervalSeconds, "5"), 5),
    [runtimeSettings]
  );
  const pollIntervalMs = useMemo(() => pollIntervalSeconds * 1000, [pollIntervalSeconds]);
  const defaultTelemetryWindow = useMemo(
    () => parseWindowSetting(settingValue(runtimeSettings, runtimeSettingKeys.defaultTelemetryWindow, "1h"), telemetryWindows, "1h"),
    [runtimeSettings]
  );
  const defaultLogWindow = useMemo(
    () => parseWindowSetting(settingValue(runtimeSettings, runtimeSettingKeys.defaultLogWindow, "1h"), telemetryWindows, "1h"),
    [runtimeSettings]
  );
  const logQueryLimit = useMemo(
    () => parseIntSetting(settingValue(runtimeSettings, runtimeSettingKeys.logQueryLimit, "120"), 120),
    [runtimeSettings]
  );
  const defaultActorID = useMemo(
    () => settingValue(runtimeSettings, runtimeSettingKeys.defaultActorID, "owner"),
    [runtimeSettings]
  );
  const defaultActionDryRun = useMemo(
    () => parseBoolSetting(settingValue(runtimeSettings, runtimeSettingKeys.defaultActionDryRun, "true"), true),
    [runtimeSettings]
  );
  const defaultUpdateDryRun = useMemo(
    () => parseBoolSetting(settingValue(runtimeSettings, runtimeSettingKeys.defaultUpdateDryRun, "true"), true),
    [runtimeSettings]
  );

  const loadRuntimeSettings = useCallback(async () => {
    try {
      const response = await fetch("/api/settings/runtime", { cache: "no-store" });
      if (!response.ok) {
        return;
      }
      const payload = (await response.json().catch(() => null)) as RuntimeSettingsPayload | null;
      setRuntimeSettings(Array.isArray(payload?.settings) ? payload.settings : []);
    } catch {
      // runtime settings unavailable — use defaults
    }
  }, []);

  useEffect(() => {
    void loadRuntimeSettings();
  }, [loadRuntimeSettings]);

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
      const base = fullStatusRef.current ?? current;
      let decision: PendingStatusPerf["decision"] = "merged";
      let nextStatus: StatusResponse | null = null;

      if (!base) {
        decision = "synthesized";
        nextStatus = {
          timestamp: live.timestamp,
          summary: {
            ...live.summary,
            connectorCount: 0,
            groupCount: 0,
            sessionCount: 0,
            auditCount: 0,
            processedJobs: 0,
            actionRunCount: 0,
            updateRunCount: 0,
            deadLetterCount: 0,
            retentionError: "",
          },
          endpoints: live.endpoints,
          assets: live.assets,
          telemetryOverview: live.telemetryOverview,
          connectors: [],
          groups: [],
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
        };
      } else if (current && areLiveSlicesEqual(current, live) && areSlowStatusSlicesEqual(current, base)) {
        decision = "unchanged";
        nextStatus = current;
      } else {
        nextStatus = {
          ...base,
          timestamp: live.timestamp,
          summary: {
            ...base.summary,
            servicesUp: live.summary.servicesUp,
            servicesTotal: live.summary.servicesTotal,
            assetCount: live.summary.assetCount,
            staleAssetCount: live.summary.staleAssetCount,
          },
          endpoints: live.endpoints,
          assets: live.assets,
          telemetryOverview: live.telemetryOverview,
        };
      }
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
      liveFetchInFlightRef.current = false;
      if (scopeID === statusScopeRef.current && requestID === liveRequestSeqRef.current) {
        setLoading(false);
      }
    }
  }, [buildGroupQuery]);

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

      if (scopeID !== statusScopeRef.current || requestID !== fullRequestSeqRef.current) {
        return;
      }

      const etag = response.headers.get("etag");
      if (etag) {
        fullStatusETagRef.current = etag;
      }

      const parseStartedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
      const payload = normalizeStatusResponse(await response.json().catch(() => null));
      const parseDurationMs = (typeof performance !== "undefined" ? performance.now() : Date.now()) - parseStartedAt;
      fullStatusRef.current = payload;

      const compareStartedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
      const current = currentStatusRef.current;
      let decision: PendingStatusPerf["decision"] = "merged";
      let nextStatus: StatusResponse;

      if (!current) {
        decision = "synthesized";
        nextStatus = payload;
      } else {
        nextStatus = {
          ...payload,
          timestamp: current.timestamp,
          summary: {
            ...payload.summary,
            servicesUp: current.summary.servicesUp,
            servicesTotal: current.summary.servicesTotal,
            assetCount: current.summary.assetCount,
            staleAssetCount: current.summary.staleAssetCount,
          },
          endpoints: current.endpoints,
          assets: current.assets,
          telemetryOverview: current.telemetryOverview,
        };
        if (areSlowStatusSlicesEqual(current, nextStatus) && areLiveSlicesEqual(current, {
          timestamp: nextStatus.timestamp,
          summary: {
            servicesUp: nextStatus.summary.servicesUp,
            servicesTotal: nextStatus.summary.servicesTotal,
            assetCount: nextStatus.summary.assetCount,
            staleAssetCount: nextStatus.summary.staleAssetCount,
          },
          endpoints: nextStatus.endpoints,
          assets: nextStatus.assets,
          telemetryOverview: nextStatus.telemetryOverview,
        })) {
          decision = "unchanged";
          nextStatus = current;
        }
      }
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
      fullFetchInFlightRef.current = false;
      if (scopeID === statusScopeRef.current && requestID === fullRequestSeqRef.current) {
        setLoading(false);
      }
    }
  }, [buildGroupQuery]);

  // fetchStatus is the public API exposed on the context — triggers both polls
  const fetchStatus = useCallback(async () => {
    await Promise.all([
      fetchLiveStatus(selectedGroupFilter),
      fetchFullStatus(selectedGroupFilter),
    ]);
  }, [fetchLiveStatus, fetchFullStatus, selectedGroupFilter]);

  useEffect(() => {
    if (previousGroupFilterRef.current === selectedGroupFilter) {
      return;
    }
    previousGroupFilterRef.current = selectedGroupFilter;
    statusScopeRef.current += 1;
    fullStatusRef.current = null;
    fullStatusETagRef.current = null;
    currentStatusRef.current = null;
    pendingPerfRef.current = null;
  }, [selectedGroupFilter]);

  // Fast polling loop (uses pollIntervalMs from runtime settings, default 5s)
  useEffect(() => {
    let timer: number | null = null;

    function startFastPolling() {
      stopFastPolling();
      void fetchLiveStatus(selectedGroupFilter);
      timer = window.setInterval(() => {
        void fetchLiveStatus(selectedGroupFilter);
      }, pollIntervalMs);
    }

    function stopFastPolling() {
      if (timer !== null) {
        window.clearInterval(timer);
        timer = null;
      }
    }

    function onVisibilityChange() {
      if (document.visibilityState === "visible") {
        startFastPolling();
      } else {
        stopFastPolling();
      }
    }

    if (document.visibilityState === "visible") {
      startFastPolling();
    }

    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      stopFastPolling();
    };
  }, [fetchLiveStatus, selectedGroupFilter, pollIntervalMs]);

  // Slow polling loop (60s, fixed — not affected by runtime poll interval setting)
  useEffect(() => {
    let timer: number | null = null;

    function startSlowPolling() {
      stopSlowPolling();
      void fetchFullStatus(selectedGroupFilter);
      timer = window.setInterval(() => {
        void fetchFullStatus(selectedGroupFilter);
      }, SLOW_POLL_MS);
    }

    function stopSlowPolling() {
      if (timer !== null) {
        window.clearInterval(timer);
        timer = null;
      }
    }

    function onVisibilityChange() {
      if (document.visibilityState === "visible") {
        startSlowPolling();
      } else {
        stopSlowPolling();
      }
    }

    if (document.visibilityState === "visible") {
      startSlowPolling();
    }

    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      stopSlowPolling();
    };
  }, [fetchFullStatus, selectedGroupFilter]);

  // WebSocket live events — triggers refetch on push notifications from backend
  const visibleRef = useRef(true);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const wsDebouncerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    let backoff = 2000;
    let closed = false;

    // Track page visibility inside this effect so we avoid a separate
    // visibilitychange listener just for visibleRef.
    visibleRef.current = document.visibilityState === "visible";
    const onVisibilityChange = () => {
      visibleRef.current = document.visibilityState === "visible";
      if (
        visibleRef.current
        && !closed
        && wsRef.current == null
        && reconnectTimer.current == null
      ) {
        void connect();
      }
    };
    document.addEventListener("visibilitychange", onVisibilityChange);

    async function resolveWsUrl(): Promise<string | null> {
      try {
        const res = await fetch("/api/ws/events", { cache: "no-store" });
        if (!res.ok) return null;
        const data = (await res.json()) as { wsUrl?: string; streamPath?: string; secure?: boolean };
        if (data.wsUrl) {
          return data.wsUrl;
        }
        if (data.streamPath) {
          return buildBrowserWsUrl(data.streamPath, { secure: data.secure });
        }
        return null;
      } catch {
        return null;
      }
    }

    async function connect() {
      if (closed) return;
      const wsUrl = await resolveWsUrl();
      if (!wsUrl || closed) return;

      try {
        const ws = new WebSocket(wsUrl);
        wsRef.current = ws;

        ws.onopen = () => {
          backoff = 2000;
        };

        ws.onmessage = (event) => {
          if (!visibleRef.current) {
            return;
          }
          try {
            const msg = JSON.parse(event.data) as { type?: string };
            const pushTypes = ["alert.fired", "alert.resolved", "heartbeat.update", "job.completed", "agent.connected", "agent.disconnected"];
            if (msg.type && pushTypes.includes(msg.type)) {
              if (wsDebouncerRef.current) clearTimeout(wsDebouncerRef.current);
              wsDebouncerRef.current = setTimeout(() => {
                wsDebouncerRef.current = null;
                // WS events only need to refresh live data — use the fast path.
                // Read groupFilterRef.current so this closure never goes stale
                // and the WS effect doesn't need selectedGroupFilter as a dep.
                void fetchLiveStatus(groupFilterRef.current);
              }, 3000);
            }
          } catch { /* ignore malformed */ }
        };

        ws.onclose = () => {
          wsRef.current = null;
          if (!closed && visibleRef.current) {
            reconnectTimer.current = setTimeout(() => {
              backoff = Math.min(backoff * 2, 30000);
              void connect();
            }, backoff);
          }
        };

        ws.onerror = () => {
          ws.close();
        };
      } catch {
        // WebSocket unavailable — polling continues as fallback
      }
    }

    void connect();

    return () => {
      closed = true;
      document.removeEventListener("visibilitychange", onVisibilityChange);
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      if (wsDebouncerRef.current) clearTimeout(wsDebouncerRef.current);
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [fetchLiveStatus]);

  const statusTimestamp = status?.timestamp ?? null;
  const statusServicesUp = status?.summary.servicesUp;
  const statusServicesTotal = status?.summary.servicesTotal;
  const statusAssetCount = status?.summary.assetCount;
  const statusStaleAssetCount = status?.summary.staleAssetCount;
  const statusConnectorCount = status?.summary.connectorCount;
  const statusGroupCount = status?.summary.groupCount;
  const statusSessionCount = status?.summary.sessionCount;
  const statusAuditCount = status?.summary.auditCount;
  const statusProcessedJobs = status?.summary.processedJobs;
  const statusActionRunCount = status?.summary.actionRunCount;
  const statusUpdateRunCount = status?.summary.updateRunCount;
  const statusDeadLetterCount = status?.summary.deadLetterCount;
  const statusRetentionError = status?.summary.retentionError;
  const statusEndpoints = status?.endpoints ?? null;
  const statusAssets = status?.assets ?? null;
  const statusTelemetryOverview = status?.telemetryOverview ?? null;
  const statusConnectors = status?.connectors ?? null;
  const statusGroups = status?.groups ?? null;
  const statusRecentLogs = status?.recentLogs ?? null;
  const statusLogSources = status?.logSources ?? null;
  const statusGroupReliability = status?.groupReliability ?? null;
  const statusActionRuns = status?.actionRuns ?? null;
  const statusUpdatePlans = status?.updatePlans ?? null;
  const statusUpdateRuns = status?.updateRuns ?? null;
  const statusDeadLetters = status?.deadLetters ?? null;
  const statusDeadLetterAnalytics = status?.deadLetterAnalytics ?? null;
  const statusSessions = status?.sessions ?? null;
  const statusRecentCommands = status?.recentCommands ?? null;
  const statusRecentAudit = status?.recentAudit ?? null;
  const statusCanonical = status?.canonical ?? null;

  const fastSummary = useMemo<FastStatusSummary | null>(() => {
    if (statusTimestamp == null) {
      return null;
    }
    return {
      servicesUp: statusServicesUp ?? 0,
      servicesTotal: statusServicesTotal ?? 0,
      assetCount: statusAssetCount ?? 0,
      staleAssetCount: statusStaleAssetCount ?? 0,
    };
  }, [
    statusAssetCount,
    statusServicesTotal,
    statusServicesUp,
    statusStaleAssetCount,
    statusTimestamp,
  ]);

  const slowSummary = useMemo<SlowStatusSummary | null>(() => {
    if (statusTimestamp == null) {
      return null;
    }
    return {
      connectorCount: statusConnectorCount ?? 0,
      groupCount: statusGroupCount ?? 0,
      sessionCount: statusSessionCount ?? 0,
      auditCount: statusAuditCount ?? 0,
      processedJobs: statusProcessedJobs ?? 0,
      actionRunCount: statusActionRunCount ?? 0,
      updateRunCount: statusUpdateRunCount ?? 0,
      deadLetterCount: statusDeadLetterCount ?? 0,
      retentionError: statusRetentionError,
    };
  }, [
    statusActionRunCount,
    statusAuditCount,
    statusConnectorCount,
    statusDeadLetterCount,
    statusGroupCount,
    statusProcessedJobs,
    statusRetentionError,
    statusSessionCount,
    statusTimestamp,
    statusUpdateRunCount,
  ]);

  const serviceStatusLabel = useMemo(() => {
    if (!fastSummary) {
      return "Connecting...";
    }
    return `${fastSummary.servicesUp}/${fastSummary.servicesTotal} online`;
  }, [fastSummary]);

  const fastStatus = useMemo<FastStatusSlice | null>(() => {
    if (!fastSummary || statusTimestamp == null || statusEndpoints == null || statusAssets == null || statusTelemetryOverview == null) {
      return null;
    }
    return {
      timestamp: statusTimestamp,
      summary: fastSummary,
      endpoints: statusEndpoints,
      assets: statusAssets,
      telemetryOverview: statusTelemetryOverview,
    };
  }, [fastSummary, statusAssets, statusEndpoints, statusTelemetryOverview, statusTimestamp]);

  const slowStatus = useMemo<SlowStatusSlice | null>(() => {
    if (
      !slowSummary
      || statusConnectors == null
      || statusGroups == null
      || statusRecentLogs == null
      || statusLogSources == null
      || statusGroupReliability == null
      || statusActionRuns == null
      || statusUpdatePlans == null
      || statusUpdateRuns == null
      || statusDeadLetters == null
      || statusDeadLetterAnalytics == null
      || statusSessions == null
      || statusRecentCommands == null
      || statusRecentAudit == null
    ) {
      return null;
    }
    return {
      summary: slowSummary,
      connectors: statusConnectors,
      groups: statusGroups,
      recentLogs: statusRecentLogs,
      logSources: statusLogSources,
      groupReliability: statusGroupReliability,
      actionRuns: statusActionRuns,
      updatePlans: statusUpdatePlans,
      updateRuns: statusUpdateRuns,
      deadLetters: statusDeadLetters,
      deadLetterAnalytics: statusDeadLetterAnalytics,
      sessions: statusSessions,
      recentCommands: statusRecentCommands,
      recentAudit: statusRecentAudit,
      canonical: statusCanonical ?? undefined,
    };
  }, [
    slowSummary,
    statusActionRuns,
    statusCanonical,
    statusConnectors,
    statusDeadLetterAnalytics,
    statusDeadLetters,
    statusGroupReliability,
    statusGroups,
    statusLogSources,
    statusRecentAudit,
    statusRecentCommands,
    statusRecentLogs,
    statusSessions,
    statusUpdatePlans,
    statusUpdateRuns,
  ]);

  const groupRows = status?.groups;
  const groupLabelByID = useMemo(() => {
    const mapped = new Map<string, string>();
    for (const group of groupRows ?? []) {
      mapped.set(group.id, group.name);
    }
    return mapped;
  }, [groupRows]);
  const assetRows = status?.assets;
  const assetNameMap = useMemo(() => {
    const next = new Map<string, string>();
    for (const asset of assetRows ?? []) {
      next.set(asset.id, asset.name);
    }
    const cached = assetNameMapRef.current;
    if (areStringMapsEqual(cached, next)) {
      return cached;
    }
    assetNameMapRef.current = next;
    return next;
  }, [assetRows]);

  const controlsValue = useMemo<StatusControlsValue>(
    () => ({
      loading,
      error,
      selectedGroupFilter,
      setSelectedGroupFilter,
      fetchStatus,
    }),
    [loading, error, selectedGroupFilter, fetchStatus],
  );

  const settingsValue = useMemo<StatusSettingsValue>(
    () => ({
      runtimeSettings,
      pollIntervalSeconds,
      defaultTelemetryWindow,
      defaultLogWindow,
      logQueryLimit,
      defaultActorID,
      defaultActionDryRun,
      defaultUpdateDryRun,
    }),
    [
      runtimeSettings,
      pollIntervalSeconds,
      defaultTelemetryWindow,
      defaultLogWindow,
      logQueryLimit,
      defaultActorID,
      defaultActionDryRun,
      defaultUpdateDryRun,
    ],
  );

  const fastValue = useMemo<StatusFastValue>(
    () => ({
      status: fastStatus,
      serviceStatusLabel,
    }),
    [fastStatus, serviceStatusLabel],
  );

  const slowValue = useMemo<StatusSlowValue>(
    () => ({
      status: slowStatus,
      groupLabelByID,
    }),
    [slowStatus, groupLabelByID],
  );

  const value = useMemo<StatusContextValue>(
    () => ({
      status,
      loading,
      error,
      selectedGroupFilter,
      setSelectedGroupFilter,
      fetchStatus,
      runtimeSettings,
      pollIntervalSeconds,
      defaultTelemetryWindow,
      defaultLogWindow,
      logQueryLimit,
      defaultActorID,
      defaultActionDryRun,
      defaultUpdateDryRun,
      serviceStatusLabel,
      groupLabelByID
    }),
    [
      status,
      loading,
      error,
      selectedGroupFilter,
      fetchStatus,
      runtimeSettings,
      pollIntervalSeconds,
      defaultTelemetryWindow,
      defaultLogWindow,
      logQueryLimit,
      defaultActorID,
      defaultActionDryRun,
      defaultUpdateDryRun,
      serviceStatusLabel,
      groupLabelByID
    ]
  );

  return (
    <StatusAssetNameMapContext.Provider value={assetNameMap}>
      <StatusControlsContext.Provider value={controlsValue}>
        <StatusSettingsContext.Provider value={settingsValue}>
          <StatusFastContext.Provider value={fastValue}>
            <StatusSlowContext.Provider value={slowValue}>
              <StatusContext.Provider value={value}>
                {children}
              </StatusContext.Provider>
            </StatusSlowContext.Provider>
          </StatusFastContext.Provider>
        </StatusSettingsContext.Provider>
      </StatusControlsContext.Provider>
    </StatusAssetNameMapContext.Provider>
  );
}

export function useStatus(): StatusContextValue {
  const context = useContext(StatusContext);
  if (!context) {
    throw new Error("useStatus must be used within a StatusProvider");
  }
  return context;
}

export function useStatusControls(): StatusControlsValue {
  const context = useContext(StatusControlsContext);
  if (!context) {
    throw new Error("useStatusControls must be used within a StatusProvider");
  }
  return context;
}

export function useStatusSettings(): StatusSettingsValue {
  const context = useContext(StatusSettingsContext);
  if (!context) {
    throw new Error("useStatusSettings must be used within a StatusProvider");
  }
  return context;
}

export function useFastStatus(): FastStatusSlice | null {
  const context = useContext(StatusFastContext);
  if (!context) {
    throw new Error("useFastStatus must be used within a StatusProvider");
  }
  return context.status;
}

export function useSlowStatus(): SlowStatusSlice | null {
  const context = useContext(StatusSlowContext);
  if (!context) {
    throw new Error("useSlowStatus must be used within a StatusProvider");
  }
  return context.status;
}

export function useServiceStatusLabel(): string {
  const context = useContext(StatusFastContext);
  if (!context) {
    throw new Error("useServiceStatusLabel must be used within a StatusProvider");
  }
  return context.serviceStatusLabel;
}

export function useGroupLabelByID(): Map<string, string> {
  const context = useContext(StatusSlowContext);
  if (!context) {
    throw new Error("useGroupLabelByID must be used within a StatusProvider");
  }
  return context.groupLabelByID;
}

export function useStatusAssetNameMap(): Map<string, string> {
  const context = useContext(StatusAssetNameMapContext);
  if (!context) {
    throw new Error("useStatusAssetNameMap must be used within a StatusProvider");
  }
  return context;
}
