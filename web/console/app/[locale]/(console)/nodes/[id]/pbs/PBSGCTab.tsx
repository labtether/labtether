"use client";

import { useMemo, useRef, useState, useCallback } from "react";

import { Badge } from "../../../../../components/ui/Badge";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import {
  formatRelativeEpoch,
  pbsTaskStatusBadge,
  usageThreshold,
  formatBytes,
} from "../pbsTabModel";
import { pbsAction, usePBSDetails } from "./usePBSData";

type Props = {
  assetId: string;
};

export function PBSGCTab({ assetId }: Props) {
  const { details, loading, error, refresh } = usePBSDetails(assetId);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const actionSeq = useRef(0);

  const sortedDatastores = useMemo(() => {
    if (!details) return [];
    const list =
      details.kind === "server"
        ? (details.datastores ?? [])
        : details.datastore
        ? [details.datastore]
        : [];
    return [...list].sort((a, b) => a.store.localeCompare(b.store));
  }, [details]);

  // GC-related tasks from the recent task list
  const gcTasks = useMemo(() => {
    const tasks = details?.tasks ?? [];
    return tasks.filter((t) =>
      ["garbage_collection", "gc"].includes(t.worker_type?.toLowerCase() ?? ""),
    );
  }, [details?.tasks]);

  const doGC = useCallback(
    async (store: string) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`gc-${store}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/datastores/${encodeURIComponent(store)}/gc`,
          "POST",
        );
        if (actionSeq.current === seq) refresh();
      } catch (err) {
        if (actionSeq.current === seq)
          setActionError(err instanceof Error ? err.message : "GC failed");
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, refresh],
  );

  if (loading && !details) {
    return <div className="p-4 text-sm text-[var(--muted)]">Loading...</div>;
  }

  if (error && !details) {
    return <div className="p-4 text-sm text-[var(--bad)]">{error}</div>;
  }

  return (
    <div className="space-y-4">
      <Card>
        <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
          <h2 className="text-sm font-medium text-[var(--text)]">Garbage Collection</h2>
          <Button size="sm" variant="ghost" onClick={refresh} disabled={loading}>
            {loading ? "Refreshing..." : "Refresh"}
          </Button>
        </div>
        {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}
        {sortedDatastores.length === 0 ? (
          <p className="text-xs text-[var(--muted)]">No datastores available.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Datastore</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Used</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Usage</th>
                  <th className="py-1 px-2 text-right text-[var(--muted)] font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {sortedDatastores.map((ds) => {
                  const threshold = usageThreshold(ds.usage_percent);
                  return (
                    <tr key={ds.store} className="border-b border-[var(--line)] border-opacity-30">
                      <td className="py-2 px-2 text-[var(--text)] font-medium">{ds.store}</td>
                      <td className="py-2 px-2 text-[var(--muted)]">
                        {formatBytes(ds.used_bytes)} / {formatBytes(ds.total_bytes)}
                      </td>
                      <td className="py-2 px-2">
                        <span
                          style={{
                            color:
                              threshold === "bad"
                                ? "var(--bad)"
                                : threshold === "warn"
                                ? "var(--warn)"
                                : "var(--ok)",
                          }}
                        >
                          {typeof ds.usage_percent === "number"
                            ? `${ds.usage_percent.toFixed(1)}%`
                            : "n/a"}
                        </span>
                      </td>
                      <td className="py-2 px-2 text-right">
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === `gc-${ds.store}`}
                          onClick={() => void doGC(ds.store)}
                        >
                          Run GC
                        </Button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {gcTasks.length > 0 && (
        <Card>
          <h2 className="text-sm font-medium text-[var(--text)] mb-3">GC History</h2>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Status</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Datastore</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Started</th>
                </tr>
              </thead>
              <tbody>
                {gcTasks.slice(0, 20).map((task) => (
                  <tr key={task.upid} className="border-b border-[var(--line)] border-opacity-30">
                    <td className="py-2 px-2">
                      <Badge status={pbsTaskStatusBadge(task.status)} size="sm" />
                    </td>
                    <td className="py-2 px-2 text-[var(--muted)]">{task.worker_id || "\u2014"}</td>
                    <td className="py-2 px-2 text-[var(--muted)]">
                      {task.starttime ? formatRelativeEpoch(task.starttime) : "n/a"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
    </div>
  );
}
