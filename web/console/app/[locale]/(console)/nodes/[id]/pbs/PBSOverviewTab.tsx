"use client";

import { useMemo } from "react";

import { Card } from "../../../../../components/ui/Card";
import {
  backupStaleness,
  formatBytes,
  formatRelativeTime,
  pbsTaskStatusBadge,
  usageThreshold,
} from "../pbsTabModel";
import { PBSServerOverviewCard } from "../PBSServerOverviewCard";
import { usePBSDetails } from "./usePBSData";

type Props = {
  assetId: string;
};

export function PBSOverviewTab({ assetId }: Props) {
  const { details, loading, error, refresh } = usePBSDetails(assetId);

  const sortedDatastores = useMemo(() => {
    if (!details) return [];
    const list = details.kind === "server"
      ? (details.datastores ?? [])
      : details.datastore
      ? [details.datastore]
      : [];
    return [...list].sort((a, b) => a.store.localeCompare(b.store));
  }, [details]);

  // Health summary
  const warnDatastores = useMemo(() => {
    return sortedDatastores.filter((ds) => {
      const nearFull = usageThreshold(ds.usage_percent) !== "ok";
      const stale = backupStaleness(
        ds.last_backup_at ? new Date(ds.last_backup_at).getTime() / 1000 : undefined,
      ) !== "ok";
      return nearFull || stale;
    });
  }, [sortedDatastores]);

  const tasks = details?.tasks ?? [];
  const recentTasks = tasks.slice(0, 50);
  const taskSuccess24h = recentTasks.filter((t) => {
    const isRecent = t.starttime && (Date.now() / 1000 - t.starttime) < 86400;
    return isRecent && pbsTaskStatusBadge(t.status) === "ok";
  }).length;
  const taskFail24h = recentTasks.filter((t) => {
    const isRecent = t.starttime && (Date.now() / 1000 - t.starttime) < 86400;
    return isRecent && pbsTaskStatusBadge(t.status) === "bad";
  }).length;

  const totalGroups = sortedDatastores.reduce((sum, ds) => sum + (ds.group_count ?? 0), 0);
  const totalSnapshots = sortedDatastores.reduce((sum, ds) => sum + (ds.snapshot_count ?? 0), 0);

  const mostRecentBackup = useMemo(() => {
    let bestAt: string | undefined;
    let bestDays: number | undefined;
    for (const ds of sortedDatastores) {
      if (typeof ds.days_since_backup === "number" && (bestDays === undefined || ds.days_since_backup < bestDays)) {
        bestDays = ds.days_since_backup;
        bestAt = ds.last_backup_at;
      }
    }
    return bestAt ? formatRelativeTime(bestAt) : "n/a";
  }, [sortedDatastores]);

  return (
    <div className="space-y-4">
      <PBSServerOverviewCard
        loading={loading}
        error={error}
        details={details}
        sortedDatastores={sortedDatastores}
        onRefresh={refresh}
      />

      {details && (
        <Card>
          <h2 className="text-sm font-medium text-[var(--text)] mb-3">Quick Stats</h2>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
            <StatTile label="Backup Groups" value={String(totalGroups)} />
            <StatTile label="Snapshots" value={String(totalSnapshots)} />
            <StatTile label="Last Backup" value={mostRecentBackup} />
            <StatTile
              label="Tasks (24h OK)"
              value={String(taskSuccess24h)}
              color="var(--ok)"
            />
            <StatTile
              label="Tasks (24h Fail)"
              value={String(taskFail24h)}
              color={taskFail24h > 0 ? "var(--bad)" : "var(--muted)"}
            />
          </div>
        </Card>
      )}

      {warnDatastores.length > 0 && (
        <Card>
          <h2 className="text-sm font-medium text-[var(--text)] mb-3">Health Warnings</h2>
          <div className="space-y-2">
            {warnDatastores.map((ds) => {
              const usageStatus = usageThreshold(ds.usage_percent);
              const stalenessStatus = backupStaleness(
                ds.last_backup_at ? new Date(ds.last_backup_at).getTime() / 1000 : undefined,
              );
              return (
                <div
                  key={ds.store}
                  className="rounded-md border border-[var(--line)] bg-[var(--surface)] p-3 space-y-1"
                >
                  <p className="text-xs font-medium text-[var(--text)]">{ds.store}</p>
                  {usageStatus !== "ok" && (
                    <p
                      className="text-xs"
                      style={{ color: usageStatus === "bad" ? "var(--bad)" : "var(--warn)" }}
                    >
                      Storage usage:{" "}
                      {typeof ds.usage_percent === "number"
                        ? `${ds.usage_percent.toFixed(1)}%`
                        : "n/a"}{" "}
                      ({formatBytes(ds.used_bytes)} / {formatBytes(ds.total_bytes)})
                    </p>
                  )}
                  {stalenessStatus !== "ok" && (
                    <p
                      className="text-xs"
                      style={{ color: stalenessStatus === "bad" ? "var(--bad)" : "var(--warn)" }}
                    >
                      Last backup:{" "}
                      {ds.last_backup_at ? formatRelativeTime(ds.last_backup_at) : "never"}
                    </p>
                  )}
                </div>
              );
            })}
          </div>
        </Card>
      )}
    </div>
  );
}

function StatTile({ label, value, color }: { label: string; value: string; color?: string }) {
  return (
    <div className="rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] px-4 py-3">
      <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">{label}</p>
      <p className="mt-1 text-sm font-medium break-all" style={{ color: color ?? "var(--text)" }}>
        {value || "--"}
      </p>
    </div>
  );
}
