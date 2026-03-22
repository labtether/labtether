"use client";

import { useCallback, useRef, useState } from "react";
import { Badge } from "../../../../../components/ui/Badge";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { formatBytes, formatRelativeTime } from "../truenasTabModel";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type TrueNASVdev = {
  name: string;
  type: string;
  status: string;
  children?: TrueNASVdev[];
};

export type TrueNASPool = {
  id: string | number;
  name: string;
  status: string;
  size?: number;
  allocated?: number;
  free?: number;
  fragmentation?: number;
  last_scrub?: string;
  topology?: TrueNASVdev[];
};

function poolStatusBadge(status: string): "ok" | "pending" | "bad" {
  const s = status.toUpperCase();
  if (s === "ONLINE") return "ok";
  if (s === "DEGRADED") return "pending";
  return "bad";
}

function VdevTree({ vdevs, depth = 0 }: { vdevs: TrueNASVdev[]; depth?: number }) {
  return (
    <ul className="space-y-0.5">
      {vdevs.map((v, idx) => (
        <li key={idx} style={{ paddingLeft: `${depth * 16}px` }}>
          <div className="flex items-center gap-2 text-xs py-0.5">
            <Badge status={poolStatusBadge(v.status)} size="sm" dot />
            <span className="text-[var(--text)]">{v.name}</span>
            <span className="text-[var(--muted)]">{v.type}</span>
          </div>
          {v.children && v.children.length > 0 ? (
            <VdevTree vdevs={v.children} depth={depth + 1} />
          ) : null}
        </li>
      ))}
    </ul>
  );
}

type Props = {
  assetId: string;
};

export function TrueNASPoolsTab({ assetId }: Props) {
  const { data: pools, loading, error, refresh } = useTrueNASList<TrueNASPool>(assetId, "pools");
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const actionSeq = useRef(0);

  const sorted = [...(Array.isArray(pools) ? pools : [])].sort((a, b) => {
    const priority = (s: string) => {
      const u = s.toUpperCase();
      if (u === "FAULTED") return 0;
      if (u === "DEGRADED") return 1;
      return 2;
    };
    return priority(a.status) - priority(b.status);
  });

  const toggleExpand = useCallback((name: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
  }, []);

  const doScrub = useCallback(
    async (poolName: string) => {
      const id = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`scrub-${poolName}`);
      try {
        await truenasAction(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/pools/${encodeURIComponent(poolName)}/scrub`,
          "POST",
        );
        if (actionSeq.current === id) {
          refresh();
        }
      } catch (err) {
        if (actionSeq.current === id) {
          setActionError(err instanceof Error ? err.message : "scrub failed");
        }
      } finally {
        if (actionSeq.current === id) {
          setActionInFlight(null);
        }
      }
    },
    [assetId, refresh],
  );

  if (loading && pools.length === 0) {
    return <Card><p className="text-sm text-[var(--muted)]">Loading pools…</p></Card>;
  }

  if (error && pools.length === 0) {
    return <Card><p className="text-sm text-[var(--bad)]">{error}</p></Card>;
  }

  return (
    <Card>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-[var(--text)]">Storage Pools</h2>
        <Button size="sm" variant="ghost" onClick={refresh} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </Button>
      </div>
      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}
      {sorted.length === 0 ? (
        <p className="text-sm text-[var(--muted)]">No pools found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Status</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Size</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Used</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Free</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Frag %</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Last Scrub</th>
                <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {sorted.map((pool) => {
                const key = pool.name;
                const isExpanded = expanded.has(key);
                const hasTopology = (pool.topology?.length ?? 0) > 0;
                return (
                  <>
                    <tr key={key}>
                      <td className="py-2 font-medium text-[var(--text)]">
                        {hasTopology ? (
                          <button
                            className="text-[var(--accent)] hover:underline mr-1"
                            onClick={() => toggleExpand(key)}
                          >
                            {isExpanded ? "▾" : "▸"}
                          </button>
                        ) : null}
                        {pool.name}
                      </td>
                      <td className="py-2">
                        <div className="flex items-center gap-2">
                          <Badge status={poolStatusBadge(pool.status)} size="sm" />
                          <span className="text-[var(--muted)]">{pool.status}</span>
                        </div>
                      </td>
                      <td className="py-2 text-[var(--muted)]">
                        {pool.size != null ? formatBytes(pool.size) : "--"}
                      </td>
                      <td className="py-2 text-[var(--muted)]">
                        {pool.allocated != null ? formatBytes(pool.allocated) : "--"}
                      </td>
                      <td className="py-2 text-[var(--muted)]">
                        {pool.free != null ? formatBytes(pool.free) : "--"}
                      </td>
                      <td className="py-2 text-[var(--muted)]">
                        {pool.fragmentation != null ? `${pool.fragmentation}%` : "--"}
                      </td>
                      <td className="py-2 text-[var(--muted)]">
                        {pool.last_scrub ? formatRelativeTime(pool.last_scrub) : "--"}
                      </td>
                      <td className="py-2 text-right">
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === `scrub-${pool.name}`}
                          onClick={() => { void doScrub(pool.name); }}
                        >
                          Scrub
                        </Button>
                      </td>
                    </tr>
                    {isExpanded && hasTopology ? (
                      <tr key={`${key}-topology`}>
                        <td colSpan={8} className="py-2 px-4 bg-[var(--surface)]">
                          <VdevTree vdevs={pool.topology!} />
                        </td>
                      </tr>
                    ) : null}
                  </>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}
