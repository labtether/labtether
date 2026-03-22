"use client";

import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { formatBytes, formatRelativeTime, usageThreshold } from "./pbsTabModel";
import type { PBSAssetDetailsResponse, PBSDatastoreSummary } from "./pbsTabModel";

type Props = {
  loading: boolean;
  error: string | null;
  details: PBSAssetDetailsResponse | null;
  sortedDatastores: PBSDatastoreSummary[];
  onRefresh: () => void;
};

export function PBSServerOverviewCard({ loading, error, details, sortedDatastores, onRefresh }: Props) {
  const totalUsed = sortedDatastores.reduce((sum, ds) => sum + (ds.used_bytes ?? 0), 0);
  const totalCapacity = sortedDatastores.reduce((sum, ds) => sum + (ds.total_bytes ?? 0), 0);
  const usagePercent = totalCapacity > 0 ? (totalUsed / totalCapacity) * 100 : 0;
  const threshold = usageThreshold(totalCapacity > 0 ? usagePercent : undefined);

  const totalGroups = sortedDatastores.reduce((sum, ds) => sum + (ds.group_count ?? 0), 0);
  const totalSnapshots = sortedDatastores.reduce((sum, ds) => sum + (ds.snapshot_count ?? 0), 0);

  const mostRecentBackup = (() => {
    let bestDays: number | undefined;
    let bestAt: string | undefined;
    for (const ds of sortedDatastores) {
      if (
        typeof ds.days_since_backup === "number" &&
        (bestDays === undefined || ds.days_since_backup < bestDays)
      ) {
        bestDays = ds.days_since_backup;
        bestAt = ds.last_backup_at;
      }
    }
    return bestAt ? formatRelativeTime(bestAt) : "n/a";
  })();

  return (
    <Card>
      {/* Row 1: Header */}
      <div className="flex items-center justify-between mb-3 gap-3 flex-wrap">
        <h2 className="text-sm font-medium text-[var(--text)]">Server Overview</h2>
        <Button size="sm" onClick={onRefresh} disabled={loading}>
          {loading ? "Refreshing..." : "Refresh"}
        </Button>
      </div>

      {/* Row 2: Error or content */}
      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : details ? (
        <div className="space-y-3">
          {/* Row 3: Metadata chips */}
          <div className="flex flex-wrap gap-2">
            <Chip label="Version" value={details.version || "n/a"} />
            <Chip label="Node" value={details.node || "localhost"} />
            <Chip label="Collector" value={details.collector_id || "n/a"} />
          </div>

          {/* Row 4: Aggregate storage bar */}
          {sortedDatastores.length > 0 && (
            <div className="space-y-1">
              <div className="flex items-center justify-between text-xs">
                <span className="text-[var(--muted)]">Total Storage</span>
                <span className="text-[var(--text)]">
                  {formatBytes(totalUsed)} / {formatBytes(totalCapacity)} ({usagePercent.toFixed(1)}%)
                </span>
              </div>
              <div className="h-2 w-full rounded-full bg-[var(--surface)]">
                <div
                  className="h-full rounded-full transition-[width,background-color] duration-[var(--dur-fast)]"
                  style={{
                    width: `${Math.min(100, usagePercent)}%`,
                    backgroundColor:
                      threshold === "bad"
                        ? "var(--bad)"
                        : threshold === "warn"
                        ? "var(--warn)"
                        : "var(--ok)",
                  }}
                />
              </div>
            </div>
          )}

          {/* Row 5: Aggregate stats */}
          {sortedDatastores.length > 0 && (
            <div className="flex flex-wrap gap-3">
              <StatChip label="Backup Groups" value={String(totalGroups)} />
              <StatChip label="Snapshots" value={String(totalSnapshots)} />
              <StatChip label="Most Recent Backup" value={mostRecentBackup} />
            </div>
          )}

          {/* Row 6: Warnings */}
          {details.warnings && details.warnings.length > 0 && (
            <ul className="space-y-1">
              {details.warnings.map((warning) => (
                <li key={warning} className="text-xs text-[var(--warn)]">
                  {warning}
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : (
        /* Row 7: Fallback */
        <p className="text-xs text-[var(--muted)]">No PBS details available.</p>
      )}
    </Card>
  );
}

function Chip({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex items-center gap-1 rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-1 text-xs">
      <span className="text-[var(--muted)]">{label}:</span>
      <span className="text-[var(--text)]">{value}</span>
    </span>
  );
}

function StatChip({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
      <p className="text-[11px] text-[var(--muted)]">{label}</p>
      <p className="text-sm font-medium text-[var(--text)]">{value}</p>
    </div>
  );
}
