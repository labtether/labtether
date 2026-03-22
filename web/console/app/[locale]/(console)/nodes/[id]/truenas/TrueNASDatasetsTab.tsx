"use client";

import { useCallback, useRef, useState } from "react";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { formatBytes } from "../truenasTabModel";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type TrueNASDataset = {
  id: string;
  name: string;
  used?: number;
  available?: number;
  quota?: number;
  compression?: string;
  mountpoint?: string;
  readonly?: boolean;
};

function datasetDepth(name: string): number {
  return (name.match(/\//g) ?? []).length;
}

function datasetShortName(name: string): string {
  const parts = name.split("/");
  return parts[parts.length - 1] ?? name;
}

type Props = {
  assetId: string;
};

export function TrueNASDatasetsTab({ assetId }: Props) {
  const { data: datasets, loading, error, refresh } = useTrueNASList<TrueNASDataset>(
    assetId,
    "datasets",
  );
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const actionSeq = useRef(0);

  const doDelete = useCallback(
    async (id: string) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`delete-${id}`);
      setConfirmDelete(null);
      try {
        await truenasAction(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/datasets/${encodeURIComponent(id)}`,
          "DELETE",
        );
        if (actionSeq.current === seq) refresh();
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

  if (loading && datasets.length === 0) {
    return <Card><p className="text-sm text-[var(--muted)]">Loading datasets…</p></Card>;
  }

  if (error && datasets.length === 0) {
    return <Card><p className="text-sm text-[var(--bad)]">{error}</p></Card>;
  }

  return (
    <Card>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-[var(--text)]">Datasets</h2>
        <Button size="sm" variant="ghost" onClick={refresh} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </Button>
      </div>
      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}
      {datasets.length === 0 ? (
        <p className="text-sm text-[var(--muted)]">No datasets found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Used</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Available</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Quota</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Compression</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Mountpoint</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">RO</th>
                <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {datasets.map((ds) => {
                const depth = datasetDepth(ds.name);
                const shortName = datasetShortName(ds.name);
                const quotaUsedPct =
                  ds.quota && ds.quota > 0 && ds.used != null
                    ? Math.min(100, Math.round((ds.used / ds.quota) * 100))
                    : null;
                return (
                  <tr key={ds.id}>
                    <td className="py-2 text-[var(--text)] max-w-[200px]">
                      <div style={{ paddingLeft: `${depth * 12}px` }}>
                        <span className="font-medium">{shortName}</span>
                      </div>
                    </td>
                    <td className="py-2 text-[var(--muted)]">
                      {ds.used != null ? formatBytes(ds.used) : "--"}
                    </td>
                    <td className="py-2 text-[var(--muted)]">
                      {ds.available != null ? formatBytes(ds.available) : "--"}
                    </td>
                    <td className="py-2 text-[var(--muted)]">
                      {ds.quota && ds.quota > 0 ? (
                        <div>
                          <span>{formatBytes(ds.quota)}</span>
                          {quotaUsedPct !== null ? (
                            <div className="mt-1 h-1 w-16 rounded-full bg-[var(--surface)] overflow-hidden">
                              <div
                                className={`h-full rounded-full ${
                                  quotaUsedPct >= 90
                                    ? "bg-[var(--bad)]"
                                    : quotaUsedPct >= 75
                                      ? "bg-[var(--warn)]"
                                      : "bg-[var(--ok)]"
                                }`}
                                style={{ width: `${quotaUsedPct}%` }}
                              />
                            </div>
                          ) : null}
                        </div>
                      ) : (
                        "--"
                      )}
                    </td>
                    <td className="py-2 text-[var(--muted)]">{ds.compression ?? "--"}</td>
                    <td className="py-2 text-[var(--muted)] max-w-[180px] truncate">
                      {ds.mountpoint ?? "--"}
                    </td>
                    <td className="py-2 text-[var(--muted)]">{ds.readonly ? "yes" : "no"}</td>
                    <td className="py-2 text-right">
                      {confirmDelete === ds.id ? (
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            loading={actionInFlight === `delete-${ds.id}`}
                            onClick={() => { void doDelete(ds.id); }}
                          >
                            Confirm
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => setConfirmDelete(null)}
                          >
                            Cancel
                          </Button>
                        </div>
                      ) : (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          onClick={() => setConfirmDelete(ds.id)}
                        >
                          Delete
                        </Button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}
