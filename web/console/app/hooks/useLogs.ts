"use client";

import { useEffect, useState } from "react";
import { useSlowStatus, useStatusControls, useStatusSettings } from "../contexts/StatusContext";
import { telemetryWindows } from "../console/models";
import type { LogEvent, LogLevel, TelemetryWindow } from "../console/models";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";
import { reportRoutePerfMetric } from "./useRoutePerfTelemetry";
import { useDocumentVisibility } from "./useDocumentVisibility";

/** While Live is on, the query runs again this often (only while the tab is visible). */
const LIVE_REFRESH_MS = 5_000;

export function useLogs() {
  const status = useSlowStatus();
  const { selectedGroupFilter } = useStatusControls();
  const { defaultLogWindow, logQueryLimit } = useStatusSettings();

  const logSources = status?.logSources ?? [];
  const recentLogs = status?.recentLogs ?? [];

  const [logWindow, setLogWindow] = useState<TelemetryWindow>(defaultLogWindow);
  const [selectedLogSource, setSelectedLogSource] = useState<string>("all");
  const [selectedAsset, setSelectedAsset] = useState<string>("all");
  const [live, setLive] = useState(false);
  const [reloadTick, setReloadTick] = useState(0);
  const visible = useDocumentVisibility();
  const [logLevel, setLogLevel] = useState<LogLevel>("all");
  const [includeHeartbeats, setIncludeHeartbeats] = useState(false);
  const [logQuery, setLogQuery] = useState<string>("");
  const [debouncedLogQuery, setDebouncedLogQuery] = useState<string>("");
  const [logEvents, setLogEvents] = useState<LogEvent[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsError, setLogsError] = useState<string | null>(null);
  const [logsLoaded, setLogsLoaded] = useState(false);
  const [hydrated, setHydrated] = useState(false);

  // Hydrate default window from settings
  useEffect(() => {
    if (hydrated) return;
    if (defaultLogWindow !== "1h" || logWindow !== "1h") {
      setLogWindow(defaultLogWindow);
    }
    setHydrated(true);
  }, [defaultLogWindow, hydrated, logWindow]);

  useEffect(() => {
    const trimmed = logQuery.trim();
    const delayMs = trimmed === "" ? 0 : 250;
    const timer = window.setTimeout(() => {
      setDebouncedLogQuery(trimmed);
    }, delayMs);
    return () => {
      window.clearTimeout(timer);
    };
  }, [logQuery]);

  useEffect(() => {
    if (!live || !visible) return;
    const timer = window.setInterval(() => setReloadTick((tick) => tick + 1), LIVE_REFRESH_MS);
    return () => window.clearInterval(timer);
  }, [live, visible]);

  useEffect(() => {
    const controller = new AbortController();
    const load = async () => {
      setLogsLoading(true);
      setLogsError(null);

      const params = new URLSearchParams();
      params.set("window", logWindow);
      params.set("limit", String(logQueryLimit));
      if (selectedLogSource !== "all") {
        params.set("source", selectedLogSource);
      }
      if (selectedAsset !== "all") {
        params.set("asset_id", selectedAsset);
      }
      if (logLevel !== "all") {
        params.set("level", logLevel);
      }
      params.set("include_heartbeats", includeHeartbeats ? "1" : "0");
      if (selectedGroupFilter === "all") {
        params.set("include_fields", "0");
      }
      if (debouncedLogQuery !== "") {
        params.set("q", debouncedLogQuery);
      }
      if (selectedGroupFilter !== "all") {
        params.set("group_id", selectedGroupFilter);
      }

      const startedAt = performance.now();
      try {
        const response = await fetch(`/api/logs/query?${params.toString()}`, {
          cache: "no-store",
          signal: controller.signal,
        });
        const payload = ensureRecord(await response.json().catch(() => null));
        if (!response.ok) {
          throw new Error(ensureString(payload?.error) || `logs fetch failed: ${response.status}`);
        }
        const events = ensureArray<LogEvent>(payload?.events);
        const eventCount = events.length;
        const requestDurationMs = performance.now() - startedAt;
        reportRoutePerfMetric({
          route: "logs",
          metric: "request.logs_query",
          durationMs: requestDurationMs,
          status: "ok",
          sampleSize: eventCount,
          metadata: {
            window: logWindow,
            source: selectedLogSource,
            level: logLevel,
            include_heartbeats: includeHeartbeats,
            group_filtered: selectedGroupFilter !== "all",
            query_active: debouncedLogQuery !== "",
          },
        });

        setLogEvents(events);
        const scheduleAfterPaint = typeof window.requestAnimationFrame === "function"
          ? window.requestAnimationFrame.bind(window)
          : (callback: FrameRequestCallback) => window.setTimeout(() => callback(performance.now()), 0);
        scheduleAfterPaint(() => {
          reportRoutePerfMetric({
            route: "logs",
            metric: "render.logs_results",
            durationMs: performance.now() - startedAt,
            status: "ok",
            sampleSize: eventCount,
            metadata: {
              window: logWindow,
              query_active: debouncedLogQuery !== "",
              group_filtered: selectedGroupFilter !== "all",
            },
          });
        });
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") {
          return;
        }
        reportRoutePerfMetric({
          route: "logs",
          metric: "request.logs_query",
          durationMs: performance.now() - startedAt,
          status: "error",
          metadata: {
            window: logWindow,
            source: selectedLogSource,
            level: logLevel,
            include_heartbeats: includeHeartbeats,
            group_filtered: selectedGroupFilter !== "all",
            query_active: debouncedLogQuery !== "",
          },
        });
        setLogEvents([]);
        setLogsError(err instanceof Error ? err.message : "logs unavailable");
      } finally {
        if (!controller.signal.aborted) {
          setLogsLoading(false);
          setLogsLoaded(true);
        }
      }
    };

    void load();
    return () => {
      controller.abort();
    };
  }, [logWindow, selectedLogSource, selectedAsset, logLevel, includeHeartbeats, debouncedLogQuery, logQueryLimit, selectedGroupFilter, reloadTick]);

  return {
    logSources,
    recentLogs,
    logWindow,
    setLogWindow,
    telemetryWindows,
    selectedLogSource,
    setSelectedLogSource,
    selectedAsset,
    setSelectedAsset,
    live,
    setLive,
    reload: () => setReloadTick((tick) => tick + 1),
    logLevel,
    setLogLevel,
    includeHeartbeats,
    setIncludeHeartbeats,
    logQuery,
    setLogQuery,
    logEvents,
    logsLoading,
    logsLoaded,
    logsError
  };
}
