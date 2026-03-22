"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import {
  backupStaleness,
  formatRelativeEpoch,
  normalizePBSGroupsResponse,
  type PBSBackupGroupEntry,
  type PBSGroupsResponse,
} from "./pbsTabModel";

type Props = {
  assetId: string;
};

export function PBSBackupCoverageCard({ assetId }: Props) {
  const [data, setData] = useState<PBSGroupsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expandedStores, setExpandedStores] = useState<Set<string>>(new Set());

  const requestSeqRef = useRef(0);
  const latestRequestRef = useRef(0);

  const fetchGroups = useCallback(async () => {
    const requestID = ++requestSeqRef.current;
    latestRequestRef.current = requestID;
    setLoading(true);
    setError(null);
    try {
      const response = await fetch(`/api/pbs/assets/${encodeURIComponent(assetId)}/groups`, {
        cache: "no-store",
      });
      const payload = normalizePBSGroupsResponse(await response.json().catch(() => null));
      if (!response.ok) {
        throw new Error(payload.error || `failed to load backup groups (${response.status})`);
      }
      if (latestRequestRef.current !== requestID) {
        return;
      }
      setData(payload);
      setExpandedStores(new Set(payload.datastores.map((ds) => ds.store)));
    } catch (err) {
      if (latestRequestRef.current !== requestID) {
        return;
      }
      setError(err instanceof Error ? err.message : "failed to load backup groups");
      setData(null);
    } finally {
      if (latestRequestRef.current === requestID) {
        setLoading(false);
      }
    }
  }, [assetId]);

  useEffect(() => {
    void fetchGroups();
  }, [fetchGroups]);

  const toggleStore = useCallback((store: string) => {
    setExpandedStores((prev) => {
      const next = new Set(prev);
      if (next.has(store)) {
        next.delete(store);
      } else {
        next.add(store);
      }
      return next;
    });
  }, []);

  const sortedDatastores = useMemo(() => {
    if (!data) return [];
    return data.datastores.map((ds) => ({
      ...ds,
      groups: [...ds.groups].sort((a, b) => (a.last_backup ?? 0) - (b.last_backup ?? 0)),
    }));
  }, [data]);

  const allGroups = data?.datastores.flatMap((ds) => ds.groups) ?? [];
  const staleCount = allGroups.filter((g) => backupStaleness(g.last_backup) !== "ok").length;
  const totalGroups = allGroups.length;

  return (
    <Card>
      <div className="flex items-center justify-between mb-3 gap-3 flex-wrap">
        <div className="flex items-center gap-3 flex-wrap min-w-0">
          <h2 className="text-sm font-medium text-[var(--text)]">Backup Coverage</h2>
          {data && totalGroups > 0 && (
            <OverallSummaryBadge staleCount={staleCount} totalGroups={totalGroups} />
          )}
        </div>
        <Button size="sm" onClick={() => { void fetchGroups(); }} disabled={loading}>
          {loading ? "Refreshing..." : "Refresh"}
        </Button>
      </div>

      {loading && !data ? (
        <p className="text-xs text-[var(--muted)]">Loading backup coverage...</p>
      ) : error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : !data || data.datastores.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">No backup groups found.</p>
      ) : (
        <div className="space-y-4">
          {sortedDatastores.map((ds) => {
            const isExpanded = expandedStores.has(ds.store);
            const dsStaleCount = ds.groups.filter((g) => backupStaleness(g.last_backup) !== "ok").length;

            return (
              <div key={ds.store}>
                <button
                  type="button"
                  className="flex items-center gap-2 w-full text-left mb-2 group"
                  onClick={() => toggleStore(ds.store)}
                >
                  <span className="text-sm font-medium text-[var(--text)] group-hover:text-[var(--accent)] transition-colors duration-[var(--dur-fast)]">
                    {ds.store}
                  </span>
                  <span className="text-xs text-[var(--muted)]">
                    {ds.groups.length} group{ds.groups.length !== 1 ? "s" : ""}
                  </span>
                  {dsStaleCount > 0 && (
                    <span className="text-xs font-medium text-[var(--warn)]">
                      {dsStaleCount} overdue
                    </span>
                  )}
                  <span className="ml-auto text-xs text-[var(--muted)] select-none">
                    {isExpanded ? "\u25BE" : "\u25B8"}
                  </span>
                </button>

                {isExpanded && (
                  ds.groups.length === 0 ? (
                    <p className="text-xs text-[var(--muted)] pl-1">No groups in this datastore.</p>
                  ) : (
                    <div className="overflow-x-auto">
                      <table className="w-full text-xs">
                        <thead>
                          <tr className="border-b border-[var(--line)]">
                            <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Type</th>
                            <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">ID</th>
                            <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Owner</th>
                            <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Snapshots</th>
                            <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Last Backup</th>
                            <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Status</th>
                          </tr>
                        </thead>
                        <tbody>
                          {ds.groups.map((group) => (
                            <GroupRow key={`${group.backup_type}-${group.backup_id}`} group={group} />
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )
                )}
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}

function OverallSummaryBadge({ staleCount, totalGroups }: { staleCount: number; totalGroups: number }) {
  if (staleCount === 0) {
    return (
      <span className="text-xs font-medium text-[var(--ok)]">
        All current ({totalGroups})
      </span>
    );
  }
  return (
    <span
      className="text-xs font-medium"
      style={{ color: staleCount > 0 ? "var(--warn)" : "var(--ok)" }}
    >
      {staleCount} overdue
    </span>
  );
}

function GroupRow({ group }: { group: PBSBackupGroupEntry }) {
  const staleness = backupStaleness(group.last_backup);

  return (
    <tr className="border-b border-[var(--line)] border-opacity-30">
      <td className="py-1.5 px-2">
        <span className="inline-block rounded bg-[var(--surface)] px-1.5 py-0.5 text-[11px] font-medium text-[var(--text)]">
          {group.backup_type}
        </span>
      </td>
      <td className="py-1.5 px-2 text-[var(--text)] font-medium">{group.backup_id}</td>
      <td className="py-1.5 px-2 text-[var(--muted)]">{group.owner || "\u2014"}</td>
      <td className="py-1.5 px-2 text-[var(--muted)]">{group.backup_count}</td>
      <td className="py-1.5 px-2" style={{ color: `var(--${staleness})` }}>
        {group.last_backup ? formatRelativeEpoch(group.last_backup) : "never"}
      </td>
      <td className="py-1.5 px-2">
        <Badge status={staleness} size="sm" />
      </td>
    </tr>
  );
}
