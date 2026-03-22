"use client";

import { useCallback, useRef, useState } from "react";
import { Badge } from "../../../../../components/ui/Badge";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { formatRelativeTime } from "../truenasTabModel";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type TrueNASReplicationTask = {
  id: string | number;
  name: string;
  direction?: string;
  source_datasets?: string[];
  target_dataset?: string;
  transport?: string;
  schedule?: string;
  last_run?: string;
  last_run_state?: string;
  enabled: boolean;
};

function replicationStateBadge(state?: string): "ok" | "pending" | "bad" {
  if (!state) return "pending";
  const s = state.toUpperCase();
  if (s === "SUCCESS" || s === "FINISHED") return "ok";
  if (s === "RUNNING" || s === "WAITING") return "pending";
  return "bad";
}

type Props = {
  assetId: string;
};

export function TrueNASReplicationTab({ assetId }: Props) {
  const { data: tasks, loading, error, refresh } = useTrueNASList<TrueNASReplicationTask>(
    assetId,
    "replication",
  );
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const actionSeq = useRef(0);

  const doRun = useCallback(
    async (id: string | number) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`run-${id}`);
      try {
        await truenasAction(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/replication/${encodeURIComponent(String(id))}/run`,
          "POST",
        );
        if (actionSeq.current === seq) refresh();
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : "run failed");
        }
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, refresh],
  );

  if (loading && tasks.length === 0) {
    return <Card><p className="text-sm text-[var(--muted)]">Loading replication tasks…</p></Card>;
  }

  if (error && tasks.length === 0) {
    return <Card><p className="text-sm text-[var(--bad)]">{error}</p></Card>;
  }

  return (
    <Card>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-[var(--text)]">Replication Tasks</h2>
        <Button size="sm" variant="ghost" onClick={refresh} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </Button>
      </div>
      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}
      {tasks.length === 0 ? (
        <p className="text-sm text-[var(--muted)]">No replication tasks found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Direction</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Source</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Target</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Transport</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Schedule</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Last Run</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Enabled</th>
                <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {tasks.map((task) => (
                <tr key={String(task.id)}>
                  <td className="py-2 font-medium text-[var(--text)]">{task.name}</td>
                  <td className="py-2 text-[var(--muted)]">{task.direction ?? "--"}</td>
                  <td className="py-2 text-[var(--muted)] max-w-[140px] truncate">
                    {task.source_datasets?.join(", ") ?? "--"}
                  </td>
                  <td className="py-2 text-[var(--muted)] max-w-[140px] truncate">
                    {task.target_dataset ?? "--"}
                  </td>
                  <td className="py-2 text-[var(--muted)]">{task.transport ?? "--"}</td>
                  <td className="py-2 text-[var(--muted)]">{task.schedule ?? "--"}</td>
                  <td className="py-2">
                    <div className="flex items-center gap-2">
                      <Badge status={replicationStateBadge(task.last_run_state)} size="sm" />
                      <span className="text-[var(--muted)]">
                        {task.last_run_state ?? "--"}
                        {task.last_run ? ` · ${formatRelativeTime(task.last_run)}` : ""}
                      </span>
                    </div>
                  </td>
                  <td className="py-2">
                    <div className="flex items-center gap-2">
                      <Badge
                        status={task.enabled ? "ok" : "bad"}
                        size="sm"
                        dot
                      />
                      <span className="text-[var(--muted)]">{task.enabled ? "Yes" : "No"}</span>
                    </div>
                  </td>
                  <td className="py-2 text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={!!actionInFlight}
                      loading={actionInFlight === `run-${task.id}`}
                      onClick={() => { void doRun(task.id); }}
                    >
                      Run Now
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}
