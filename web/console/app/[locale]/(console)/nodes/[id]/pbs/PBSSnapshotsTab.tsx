"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Badge } from "../../../../../components/ui/Badge";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import {
  formatBytes,
  formatRelativeEpoch,
  normalizePBSSnapshotsResponse,
  type PBSSnapshotEntry,
} from "../pbsTabModel";
import { pbsAction } from "./usePBSData";

type Props = {
  assetId: string;
};

type StoreFilter = {
  store: string;
  backupType: string;
  backupId: string;
};

export function PBSSnapshotsTab({ assetId }: Props) {
  const [snapshots, setSnapshots] = useState<PBSSnapshotEntry[]>([]);
  const [currentStore, setCurrentStore] = useState("");
  const [storeInput, setStoreInput] = useState("");
  const [typeInput, setTypeInput] = useState("vm");
  const [idInput, setIdInput] = useState("");

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirmForgetId, setConfirmForgetId] = useState<string | null>(null);
  const [confirmBulkForget, setConfirmBulkForget] = useState(false);

  const seqRef = useRef(0);
  const latestRef = useRef(0);
  const actionSeq = useRef(0);

  const fetchSnapshots = useCallback(
    async (filter: StoreFilter) => {
      const id = ++seqRef.current;
      latestRef.current = id;
      setLoading(true);
      setError(null);
      const params = new URLSearchParams({ store: filter.store });
      if (filter.backupType) params.set("type", filter.backupType);
      if (filter.backupId) params.set("id", filter.backupId);
      try {
        const response = await fetch(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/snapshots?${params.toString()}`,
          { cache: "no-store" },
        );
        const payload = normalizePBSSnapshotsResponse(await response.json().catch(() => null));
        if (!response.ok) {
          throw new Error(payload.error || `failed to load snapshots (${response.status})`);
        }
        if (latestRef.current !== id) return;
        setSnapshots(payload.snapshots);
        setCurrentStore(filter.store);
        setSelected(new Set());
      } catch (err) {
        if (latestRef.current !== id) return;
        setError(err instanceof Error ? err.message : "failed to load snapshots");
        setSnapshots([]);
      } finally {
        if (latestRef.current === id) setLoading(false);
      }
    },
    [assetId],
  );

  const sorted = useMemo(() => [...snapshots].sort((a, b) => b.backup_time - a.backup_time), [snapshots]);

  const snapshotKey = (snap: PBSSnapshotEntry) =>
    `${snap.backup_type}/${snap.backup_id}/${snap.backup_time}`;

  const toggleSelect = useCallback((key: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  const doVerify = useCallback(
    async (snap: PBSSnapshotEntry) => {
      const key = snapshotKey(snap);
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`verify-${key}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/snapshots/verify`,
          "POST",
          {
            store: currentStore,
            backup_type: snap.backup_type,
            backup_id: snap.backup_id,
            backup_time: snap.backup_time,
          },
        );
        if (actionSeq.current === seq) {
          void fetchSnapshots({ store: currentStore, backupType: typeInput, backupId: idInput });
        }
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : "verify failed");
        }
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, currentStore, fetchSnapshots, idInput, typeInput],
  );

  const doForget = useCallback(
    async (snap: PBSSnapshotEntry) => {
      const key = snapshotKey(snap);
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`forget-${key}`);
      setConfirmForgetId(null);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/snapshots/forget`,
          "POST",
          {
            store: currentStore,
            backup_type: snap.backup_type,
            backup_id: snap.backup_id,
            backup_time: snap.backup_time,
          },
        );
        if (actionSeq.current === seq) {
          setSelected((prev) => {
            const next = new Set(prev);
            next.delete(key);
            return next;
          });
          void fetchSnapshots({ store: currentStore, backupType: typeInput, backupId: idInput });
        }
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : "forget failed");
        }
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, currentStore, fetchSnapshots, idInput, typeInput],
  );

  const doBulkForget = useCallback(async () => {
    const seq = ++actionSeq.current;
    setActionError(null);
    setActionInFlight("bulk-forget");
    setConfirmBulkForget(false);
    const keys = Array.from(selected);
    const errors: string[] = [];
    for (const key of keys) {
      const [bType, bId, bTimeStr] = key.split("/");
      const bTime = parseInt(bTimeStr ?? "0", 10);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/snapshots/forget`,
          "POST",
          { store: currentStore, backup_type: bType, backup_id: bId, backup_time: bTime },
        );
      } catch (err) {
        errors.push(err instanceof Error ? err.message : key);
      }
    }
    if (actionSeq.current !== seq) return;
    setActionInFlight(null);
    setSelected(new Set());
    if (errors.length > 0) setActionError(`Failed: ${errors.join(", ")}`);
    void fetchSnapshots({ store: currentStore, backupType: typeInput, backupId: idInput });
  }, [assetId, currentStore, fetchSnapshots, idInput, selected, typeInput]);

  return (
    <Card>
      <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">Snapshots</h2>
      </div>

      {/* Filter controls */}
      <div className="flex flex-wrap gap-2 items-end mb-4">
        <label className="flex flex-col gap-1">
          <span className="text-xs text-[var(--muted)]">Datastore</span>
          <input
            className="rounded border border-[var(--line)] bg-[var(--panel-glass)] text-xs text-[var(--text)] px-2 py-1"
            value={storeInput}
            onChange={(e) => setStoreInput(e.target.value)}
            placeholder="store-name"
          />
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-xs text-[var(--muted)]">Type</span>
          <select
            className="rounded border border-[var(--line)] bg-[var(--panel-glass)] text-xs text-[var(--text)] px-2 py-1"
            value={typeInput}
            onChange={(e) => setTypeInput(e.target.value)}
          >
            <option value="vm">vm</option>
            <option value="ct">ct</option>
            <option value="host">host</option>
          </select>
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-xs text-[var(--muted)]">ID (optional)</span>
          <input
            className="rounded border border-[var(--line)] bg-[var(--panel-glass)] text-xs text-[var(--text)] px-2 py-1"
            value={idInput}
            onChange={(e) => setIdInput(e.target.value)}
            placeholder="100"
          />
        </label>
        <Button
          size="sm"
          disabled={!storeInput || loading}
          loading={loading}
          onClick={() =>
            void fetchSnapshots({ store: storeInput, backupType: typeInput, backupId: idInput })
          }
        >
          Load
        </Button>
        {snapshots.length > 0 && (
          <Button
            size="sm"
            variant="ghost"
            disabled={loading}
            onClick={() =>
              void fetchSnapshots({ store: storeInput, backupType: typeInput, backupId: idInput })
            }
          >
            Refresh
          </Button>
        )}
        {selected.size > 0 &&
          (confirmBulkForget ? (
            <>
              <Button
                size="sm"
                variant="danger"
                loading={actionInFlight === "bulk-forget"}
                onClick={() => { void doBulkForget(); }}
              >
                Confirm Forget ({selected.size})
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setConfirmBulkForget(false)}>
                Cancel
              </Button>
            </>
          ) : (
            <Button
              size="sm"
              variant="ghost"
              disabled={!!actionInFlight}
              onClick={() => setConfirmBulkForget(true)}
            >
              Forget Selected ({selected.size})
            </Button>
          ))}
      </div>

      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}

      {!currentStore && !loading ? (
        <p className="text-xs text-[var(--muted)]">Enter a datastore name and click Load to browse snapshots.</p>
      ) : loading && snapshots.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">Loading snapshots...</p>
      ) : error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : sorted.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">No snapshots found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-2 text-left font-medium text-[var(--muted)] w-6">
                  <span className="sr-only">Select</span>
                </th>
                <th className="py-2 px-2 text-left font-medium text-[var(--muted)]">Time</th>
                <th className="py-2 px-2 text-left font-medium text-[var(--muted)]">Type/ID</th>
                <th className="py-2 px-2 text-left font-medium text-[var(--muted)]">Size</th>
                <th className="py-2 px-2 text-left font-medium text-[var(--muted)]">Protected</th>
                <th className="py-2 px-2 text-left font-medium text-[var(--muted)]">Verification</th>
                <th className="py-2 px-2 text-right font-medium text-[var(--muted)]">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {sorted.map((snap) => {
                const key = snapshotKey(snap);
                const verificationStatus =
                  snap.verification?.state === "ok"
                    ? "ok"
                    : snap.verification?.state === "failed"
                    ? "bad"
                    : snap.verification?.state
                    ? "pending"
                    : null;
                return (
                  <tr key={key}>
                    <td className="py-2">
                      <input
                        type="checkbox"
                        checked={selected.has(key)}
                        onChange={() => toggleSelect(key)}
                        className="rounded"
                      />
                    </td>
                    <td className="py-2 px-2 text-[var(--text)]">{formatRelativeEpoch(snap.backup_time)}</td>
                    <td className="py-2 px-2 text-[var(--muted)]">
                      {snap.backup_type}/{snap.backup_id}
                    </td>
                    <td className="py-2 px-2 text-[var(--muted)]">
                      {snap.size !== undefined ? formatBytes(snap.size) : "n/a"}
                    </td>
                    <td className="py-2 px-2">
                      {snap.protected ? (
                        <span className="text-[var(--ok)] text-[11px]">yes</span>
                      ) : (
                        <span className="text-[var(--muted)]">&mdash;</span>
                      )}
                    </td>
                    <td className="py-2 px-2">
                      {verificationStatus ? (
                        <Badge status={verificationStatus} size="sm" />
                      ) : (
                        <span className="text-[var(--muted)]">none</span>
                      )}
                    </td>
                    <td className="py-2 px-2 text-right">
                      {confirmForgetId === key ? (
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            size="sm"
                            variant="danger"
                            loading={actionInFlight === `forget-${key}`}
                            onClick={() => { void doForget(snap); }}
                          >
                            Confirm
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => setConfirmForgetId(null)}
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
                            loading={actionInFlight === `verify-${key}`}
                            onClick={() => { void doVerify(snap); }}
                          >
                            Verify
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={!!actionInFlight || snap.protected}
                            onClick={() => setConfirmForgetId(key)}
                          >
                            Forget
                          </Button>
                        </div>
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
