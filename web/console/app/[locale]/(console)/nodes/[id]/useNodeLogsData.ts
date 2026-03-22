"use client";

import { useCallback, useEffect, useState } from "react";
import type { LogEvent, TelemetryWindow } from "../../../../console/models";
import { ensureArray, ensureRecord, ensureString } from "../../../../lib/responseGuards";

type UseNodeLogsDataArgs = {
  activeTab: string;
  nodeId: string;
};

export function useNodeLogsData({ activeTab, nodeId }: UseNodeLogsDataArgs) {
  const [logEvents, setLogEvents] = useState<LogEvent[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsError, setLogsError] = useState<string | null>(null);
  const [logLevelFilter, setLogLevelFilter] = useState<string>("all");
  const [logMode, setLogMode] = useState<"stored" | "journal">("stored");
  const [logWindow, setLogWindow] = useState<TelemetryWindow>("1h");
  const [journalSince, setJournalSince] = useState("1h ago");
  const [journalUntil, setJournalUntil] = useState("");
  const [journalUnit, setJournalUnit] = useState("");
  const [journalPriority, setJournalPriority] = useState("all");
  const [journalQuery, setJournalQuery] = useState("");
  const [journalLiveTail, setJournalLiveTail] = useState(false);
  const [logsRefreshTick, setLogsRefreshTick] = useState(0);

  useEffect(() => {
    if (activeTab !== "logs" || !nodeId) return;
    const controller = new AbortController();
    setLogsLoading(true);
    setLogsError(null);
    const load = async () => {
      try {
        if (logMode === "stored") {
          const params = new URLSearchParams({ asset_id: nodeId, limit: "150", window: logWindow });
          const res = await fetch(`/api/logs/query?${params.toString()}`, {
            cache: "no-store",
            signal: controller.signal,
          });
          const data = ensureRecord(await res.json().catch(() => null));
          if (!res.ok) {
            throw new Error(ensureString(data?.error) || `failed to load logs (${res.status})`);
          }
          setLogEvents(ensureArray<LogEvent>(data?.events));
        } else {
          const params = new URLSearchParams({ limit: journalLiveTail ? "200" : "150" });
          const trimmedSince = journalSince.trim();
          if (trimmedSince !== "") {
            params.set("since", trimmedSince);
          } else if (journalLiveTail) {
            params.set("since", "30s ago");
          }
          if (!journalLiveTail && journalUntil.trim() !== "") params.set("until", journalUntil.trim());
          if (journalUnit.trim() !== "") params.set("unit", journalUnit.trim());
          if (journalPriority.trim() !== "" && journalPriority !== "all") {
            params.set("priority", journalPriority.trim());
          }
          if (journalQuery.trim() !== "") params.set("q", journalQuery.trim());
          const res = await fetch(`/api/logs/journal/${encodeURIComponent(nodeId)}?${params.toString()}`, {
            cache: "no-store",
            signal: controller.signal,
          });
          const data = ensureRecord(await res.json().catch(() => null));
          if (!res.ok) {
            throw new Error(ensureString(data?.error) || `failed to load journal logs (${res.status})`);
          }
          const incoming = ensureArray<LogEvent>(data?.entries);
          if (journalLiveTail) {
            setLogEvents((existing) => mergeLiveTailEntries(existing, incoming, 400));
          } else {
            setLogEvents(incoming);
          }
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setLogEvents([]);
        setLogsError(err instanceof Error ? err.message : "failed to load logs");
      } finally {
        if (!controller.signal.aborted) setLogsLoading(false);
      }
    };
    void load();
    return () => { controller.abort(); };
  }, [activeTab, nodeId, logMode, logWindow, journalSince, journalUntil, journalUnit, journalPriority, journalQuery, journalLiveTail, logsRefreshTick]);

  useEffect(() => {
    if (activeTab !== "logs" || logMode !== "journal" || !journalLiveTail) {
      return;
    }
    const interval = setInterval(() => {
      // Pause polling when the tab is hidden to save resources.
      if (document.visibilityState === "hidden") return;
      setLogsRefreshTick((value) => value + 1);
    }, 2_000);
    return () => clearInterval(interval);
  }, [activeTab, logMode, journalLiveTail]);

  const refreshLogs = useCallback(() => {
    setLogsRefreshTick((value) => value + 1);
  }, []);

  return {
    logEvents,
    logsLoading,
    logsError,
    logLevelFilter,
    setLogLevelFilter,
    logMode,
    setLogMode,
    logWindow,
    setLogWindow,
    journalSince,
    setJournalSince,
    journalUntil,
    setJournalUntil,
    journalUnit,
    setJournalUnit,
    journalPriority,
    setJournalPriority,
    journalQuery,
    setJournalQuery,
    journalLiveTail,
    setJournalLiveTail,
    refreshLogs,
  };
}

function mergeLiveTailEntries(existing: LogEvent[], incoming: LogEvent[], maxEntries: number): LogEvent[] {
  const byKey = new Map<string, LogEvent>();

  for (const event of [...incoming, ...existing]) {
    const fallback = `${event.timestamp}:${event.source}:${event.level}:${event.message}`;
    const key = event.id.trim() !== "" ? event.id : fallback;
    if (!byKey.has(key)) {
      byKey.set(key, event);
    }
  }

  return [...byKey.values()]
    .sort((left, right) => new Date(right.timestamp).getTime() - new Date(left.timestamp).getTime())
    .slice(0, maxEntries);
}
