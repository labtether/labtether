"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { formatBytes, formatRelativeTime } from "../truenasTabModel";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type TrueNASSnapshot = {
  id: string;
  name: string;
  dataset: string;
  snapshot_name: string;
  created?: string;
  used?: number;
  referenced?: number;
};

type Props = {
  assetId: string;
};

export function TrueNASSnapshotsTab({ assetId }: Props) {
  const { data: snapshots, loading, error, refresh } = useTrueNASList<TrueNASSnapshot>(
    assetId,
    "snapshots",
  );

  const [datasetFilter, setDatasetFilter] = useState<string>("__all__");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [confirmDeleteBulk, setConfirmDeleteBulk] = useState(false);
  const actionSeq = useRef(0);

  const datasets = useMemo(() => {
    const seen = new Set<string>();
    for (const snap of snapshots) {
      seen.add(snap.dataset);
    }
    return Array.from(seen).sort();
  }, [snapshots]);

  const filtered = useMemo(() => {
    if (datasetFilter === "__all__") return snapshots;
    return snapshots.filter((s) => s.dataset === datasetFilter);
  }, [snapshots, datasetFilter]);

  const toggleSelect = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const doDelete = useCallback(
    async (id: string) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`delete-${id}`);
      setConfirmDeleteId(null);
      try {
        await truenasAction(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/snapshots/${encodeURIComponent(id)}`,
          "DELETE",
        );
        if (actionSeq.current === seq) {
          setSelected((prev) => {
            const next = new Set(prev);
            next.delete(id);
            return next;
          });
          refresh();
        }
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : "delete failed");
        }
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, refresh],
  );

  const doBulkDelete = useCallback(async () => {
    const seq = ++actionSeq.current;
    setActionError(null);
    setActionInFlight("bulk-delete");
    setConfirmDeleteBulk(false);
    const ids = Array.from(selected);
    const errors: string[] = [];
    for (const id of ids) {
      try {
        await truenasAction(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/snapshots/${encodeURIComponent(id)}`,
          "DELETE",
        );
      } catch (err) {
        errors.push(err instanceof Error ? err.message : id);
      }
    }
    if (actionSeq.current !== seq) return;
    setActionInFlight(null);
    setSelected(new Set());
    if (errors.length > 0) {
      setActionError(`Failed to delete: ${errors.join(", ")}`);
    }
    refresh();
  }, [assetId, selected, refresh]);

  const doRollback = useCallback(
    async (id: string) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`rollback-${id}`);
      try {
        await truenasAction(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/snapshots/${encodeURIComponent(id)}/rollback`,
          "POST",
        );
        if (actionSeq.current === seq) refresh();
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : "rollback failed");
        }
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, refresh],
  );

  if (loading && snapshots.length === 0) {
    return <Card><p className="text-sm text-[var(--muted)]">Loading snapshots…</p></Card>;
  }

  if (error && snapshots.length === 0) {
    return <Card><p className="text-sm text-[var(--bad)]">{error}</p></Card>;
  }

  return (
    <Card>
      <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">Snapshots</h2>
        <div className="flex items-center gap-2 flex-wrap">
          {datasets.length > 0 ? (
            <select
              value={datasetFilter}
              onChange={(e) => setDatasetFilter(e.target.value)}
              className="rounded border border-[var(--line)] bg-[var(--surface)] text-xs text-[var(--text)] px-2 py-1"
            >
              <option value="__all__">All datasets</option>
              {datasets.map((ds) => (
                <option key={ds} value={ds}>
                  {ds}
                </option>
              ))}
            </select>
          ) : null}
          {selected.size > 0 ? (
            confirmDeleteBulk ? (
              <>
                <Button
                  size="sm"
                  variant="ghost"
                  loading={actionInFlight === "bulk-delete"}
                  onClick={() => { void doBulkDelete(); }}
                >
                  Confirm Delete ({selected.size})
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setConfirmDeleteBulk(false)}>
                  Cancel
                </Button>
              </>
            ) : (
              <Button
                size="sm"
                variant="ghost"
                disabled={!!actionInFlight}
                onClick={() => setConfirmDeleteBulk(true)}
              >
                Delete Selected ({selected.size})
              </Button>
            )
          ) : null}
          <Button size="sm" variant="ghost" onClick={refresh} disabled={loading}>
            {loading ? "Refreshing…" : "Refresh"}
          </Button>
        </div>
      </div>
      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}
      {filtered.length === 0 ? (
        <p className="text-sm text-[var(--muted)]">No snapshots found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-2 text-left font-medium text-[var(--muted)] w-6">
                  <span className="sr-only">Select</span>
                </th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Dataset</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Created</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Used</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Referenced</th>
                <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {filtered.map((snap) => (
                <tr key={snap.id}>
                  <td className="py-2">
                    <input
                      type="checkbox"
                      checked={selected.has(snap.id)}
                      onChange={() => toggleSelect(snap.id)}
                      className="rounded"
                    />
                  </td>
                  <td className="py-2 font-medium text-[var(--text)]">{snap.name}</td>
                  <td className="py-2 text-[var(--muted)]">{snap.dataset}</td>
                  <td className="py-2 text-[var(--muted)]">
                    {snap.created ? formatRelativeTime(snap.created) : "--"}
                  </td>
                  <td className="py-2 text-[var(--muted)]">
                    {snap.used != null ? formatBytes(snap.used) : "--"}
                  </td>
                  <td className="py-2 text-[var(--muted)]">
                    {snap.referenced != null ? formatBytes(snap.referenced) : "--"}
                  </td>
                  <td className="py-2 text-right">
                    {confirmDeleteId === snap.id ? (
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          loading={actionInFlight === `delete-${snap.id}`}
                          onClick={() => { void doDelete(snap.id); }}
                        >
                          Confirm
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setConfirmDeleteId(null)}
                        >
                          Cancel
                        </Button>
                      </div>
                    ) : (
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === `rollback-${snap.id}`}
                          onClick={() => { void doRollback(snap.id); }}
                        >
                          Rollback
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          onClick={() => setConfirmDeleteId(snap.id)}
                        >
                          Delete
                        </Button>
                      </div>
                    )}
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
