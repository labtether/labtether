"use client";

import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { formatBytes, formatRelativeTime, pbsStatusBadge } from "./pbsTabModel";
import type { PBSAssetDetailsResponse, PBSDatastoreSummary } from "./pbsTabModel";

type PBSHealthCardProps = {
  loading: boolean;
  error: string | null;
  details: PBSAssetDetailsResponse | null;
  sortedDatastores: PBSDatastoreSummary[];
  onRefresh: () => void;
};

export function PBSHealthCard({ loading, error, details, sortedDatastores, onRefresh }: PBSHealthCardProps) {
  return (
    <Card>
      <div className="flex items-center justify-between mb-3 gap-3 flex-wrap">
        <h2 className="text-sm font-medium text-[var(--text)]">PBS Health</h2>
        <Button size="sm" onClick={onRefresh} disabled={loading}>
          {loading ? "Refreshing..." : "Refresh"}
        </Button>
      </div>
      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : details ? (
        <div className="space-y-3">
          <div className="flex flex-wrap gap-2">
            <Chip label="Kind" value={details.kind || "unknown"} />
            <Chip label="Node" value={details.node || "localhost"} />
            <Chip label="Version" value={details.version || "n/a"} />
            <Chip label="Collector" value={details.collector_id || "n/a"} />
          </div>
          {details.warnings && details.warnings.length > 0 ? (
            <ul className="space-y-1">
              {details.warnings.map((warning) => (
                <li key={warning} className="text-xs text-[var(--warn)]">{warning}</li>
              ))}
            </ul>
          ) : null}
          {sortedDatastores.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-[var(--line)]">
                    <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Datastore</th>
                    <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Status</th>
                    <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Usage</th>
                    <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Groups</th>
                    <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Snapshots</th>
                    <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Last Backup</th>
                  </tr>
                </thead>
                <tbody>
                  {sortedDatastores.map((store) => (
                    <tr key={store.store} className="border-b border-[var(--line)] border-opacity-30">
                      <td className="py-1 px-2 text-[var(--text)] font-medium">{store.store}</td>
                      <td className="py-1 px-2">
                        <Badge status={pbsStatusBadge(store.status)} size="sm" />
                      </td>
                      <td className="py-1 px-2 text-[var(--muted)]">
                        {typeof store.usage_percent === "number"
                          ? `${store.usage_percent.toFixed(1)}% (${formatBytes(store.used_bytes)} / ${formatBytes(store.total_bytes)})`
                          : "n/a"}
                      </td>
                      <td className="py-1 px-2 text-[var(--muted)]">{store.group_count ?? 0}</td>
                      <td className="py-1 px-2 text-[var(--muted)]">{store.snapshot_count ?? 0}</td>
                      <td className="py-1 px-2 text-[var(--muted)]">
                        {store.last_backup_at ? formatRelativeTime(store.last_backup_at) : "n/a"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p className="text-xs text-[var(--muted)]">No datastore health data returned.</p>
          )}
        </div>
      ) : (
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
