"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { PBSRecentTasksCard } from "../PBSRecentTasksCard";
import { PBSTaskDetailsCard } from "../PBSTaskDetailsCard";
import { usePBSDetails } from "./usePBSData";
import {
  normalizePBSTaskLogLines,
  normalizePBSTaskStatus,
  type PBSTask,
  type PBSTaskLogLine,
  type PBSTaskStatus,
} from "../pbsTabModel";

type Props = {
  assetId: string;
};

export function PBSTasksTab({ assetId }: Props) {
  const { details, refresh: refreshDetails } = usePBSDetails(assetId);

  const collectorID = details?.collector_id?.trim() ?? "";
  const nodeFallback = details?.node?.trim() || "localhost";
  // Memoize allTasks so its reference is stable between renders when details
  // hasn't changed. A plain `details?.tasks ?? []` expression would create a
  // new array reference on every render and invalidate the useMemo hooks below.
  const allTasks = useMemo(() => details?.tasks ?? [], [details]);

  const [typeFilter, setTypeFilter] = useState<string>("__all__");
  const [statusFilter, setStatusFilter] = useState<"all" | "errors" | "running">("all");

  const [selectedTask, setSelectedTask] = useState<PBSTask | null>(null);
  const [taskStatus, setTaskStatus] = useState<PBSTaskStatus | null>(null);
  const [taskLog, setTaskLog] = useState<PBSTaskLogLine[]>([]);
  const [taskLoading, setTaskLoading] = useState(false);
  const [taskError, setTaskError] = useState<string | null>(null);
  const [stopError, setStopError] = useState<string | null>(null);
  const [stoppingTask, setStoppingTask] = useState(false);

  const selectedTaskRef = useRef<PBSTask | null>(null);
  const taskSeqRef = useRef(0);
  const latestTaskRef = useRef(0);

  useEffect(() => {
    selectedTaskRef.current = selectedTask;
  }, [selectedTask]);

  const taskTypes = useMemo(() => {
    const seen = new Set<string>();
    for (const t of allTasks) seen.add(t.worker_type || "unknown");
    return Array.from(seen).sort();
  }, [allTasks]);

  const filteredTasks = useMemo(() => {
    return allTasks.filter((t) => {
      const matchType = typeFilter === "__all__" || t.worker_type === typeFilter;
      const matchStatus =
        statusFilter === "all"
          ? true
          : statusFilter === "errors"
          ? (t.status ?? "").toLowerCase().includes("error") ||
            (t.status ?? "").toLowerCase().includes("fail")
          : (t.status ?? "").toLowerCase().includes("run") ||
            (t.status ?? "").toLowerCase().includes("active");
      return matchType && matchStatus;
    });
  }, [allTasks, typeFilter, statusFilter]);

  const fetchTaskDetails = useCallback(
    async (task: PBSTask) => {
      const id = ++taskSeqRef.current;
      latestTaskRef.current = id;
      const node = task.node?.trim() || nodeFallback;
      const upid = task.upid?.trim() ?? "";
      if (!node || !upid) return;
      const query = collectorID ? `?collector_id=${encodeURIComponent(collectorID)}` : "";
      setTaskLoading(true);
      setTaskError(null);
      setStopError(null);
      try {
        const [statusRes, logRes] = await Promise.all([
          fetch(`/api/pbs/tasks/${encodeURIComponent(node)}/${encodeURIComponent(upid)}/status${query}`, {
            cache: "no-store",
          }),
          fetch(`/api/pbs/tasks/${encodeURIComponent(node)}/${encodeURIComponent(upid)}/log${query}`, {
            cache: "no-store",
          }),
        ]);
        const statusPayload = (await statusRes.json().catch(() => null)) as { task?: unknown; error?: string } | null;
        const logPayload = (await logRes.json().catch(() => null)) as { lines?: unknown; error?: string } | null;
        if (!statusRes.ok) throw new Error(statusPayload?.error || `failed to load task status (${statusRes.status})`);
        if (!logRes.ok) throw new Error(logPayload?.error || `failed to load task log (${logRes.status})`);
        if (latestTaskRef.current !== id) return;
        setTaskStatus(normalizePBSTaskStatus(statusPayload?.task));
        setTaskLog(normalizePBSTaskLogLines(logPayload?.lines));
      } catch (err) {
        if (latestTaskRef.current !== id) return;
        setTaskError(err instanceof Error ? err.message : "failed to load task details");
        setTaskStatus(null);
        setTaskLog([]);
      } finally {
        if (latestTaskRef.current === id) setTaskLoading(false);
      }
    },
    [collectorID, nodeFallback],
  );

  const handleStopTask = useCallback(async () => {
    if (!selectedTask) return;
    const node = selectedTask.node?.trim() || nodeFallback;
    const upid = selectedTask.upid?.trim() ?? "";
    if (!node || !upid) return;
    const query = collectorID ? `?collector_id=${encodeURIComponent(collectorID)}` : "";
    setStoppingTask(true);
    setTaskError(null);
    setStopError(null);
    try {
      const response = await fetch(
        `/api/pbs/tasks/${encodeURIComponent(node)}/${encodeURIComponent(upid)}/stop${query}`,
        { method: "POST" },
      );
      const payload = (await response.json().catch(() => null)) as { error?: string } | null;
      if (!response.ok) {
        setStopError(payload?.error || `failed to stop task (${response.status})`);
        return;
      }
      await fetchTaskDetails(selectedTask);
      refreshDetails();
    } catch (err) {
      setStopError(err instanceof Error ? err.message : "failed to stop task");
    } finally {
      setStoppingTask(false);
    }
  }, [collectorID, fetchTaskDetails, nodeFallback, refreshDetails, selectedTask]);

  useEffect(() => {
    if (!selectedTask) return;
    void fetchTaskDetails(selectedTask);
  }, [selectedTask, fetchTaskDetails]);

  const visibleTaskError = stopError ?? taskError;

  return (
    <div className="space-y-4">
      {/* Filters */}
      <div className="flex flex-wrap gap-2 items-center">
        <select
          className="rounded border border-[var(--line)] bg-[var(--surface)] text-xs text-[var(--text)] px-2 py-1"
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
        >
          <option value="__all__">All types</option>
          {taskTypes.map((t) => (
            <option key={t} value={t}>{t}</option>
          ))}
        </select>
        <div className="flex gap-1">
          {(["all", "errors", "running"] as const).map((tab) => (
            <button
              key={tab}
              className={`rounded-md px-2.5 py-1 text-xs transition-colors ${
                statusFilter === tab
                  ? "bg-[var(--hover)] text-[var(--text)] font-medium"
                  : "text-[var(--muted)] hover:text-[var(--text)]"
              }`}
              onClick={() => setStatusFilter(tab)}
            >
              {tab === "all" ? "All" : tab === "errors" ? "Errors" : "Running"}
            </button>
          ))}
        </div>
      </div>

      <PBSRecentTasksCard
        tasks={filteredTasks}
        selectedTask={selectedTask}
        onSelectTask={(task) => {
          setStopError(null);
          setSelectedTask(task);
        }}
      />

      {selectedTask ? (
        <PBSTaskDetailsCard
          selectedTask={selectedTask}
          taskStatus={taskStatus}
          taskLog={taskLog}
          taskLoading={taskLoading}
          stoppingTask={stoppingTask}
          error={visibleTaskError}
          nodeFallback={nodeFallback}
          onRefresh={() => { void fetchTaskDetails(selectedTask); }}
          onStopTask={() => { void handleStopTask(); }}
        />
      ) : null}
    </div>
  );
}
