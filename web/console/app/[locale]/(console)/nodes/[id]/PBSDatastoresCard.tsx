"use client";

import { Fragment, useState } from "react";

import { Badge } from "../../../../components/ui/Badge";
import { Card } from "../../../../components/ui/Card";
import {
  formatBytes,
  formatRelativeTime,
  pbsStatusBadge,
  usageThreshold,
  type PBSDatastoreSummary,
} from "./pbsTabModel";
import { PBSDatastoreDrilldown } from "./PBSDatastoreDrilldown";

type Props = {
  datastores: PBSDatastoreSummary[];
  assetId: string;
};

export function PBSDatastoresCard({ datastores, assetId }: Props) {
  const [expandedStore, setExpandedStore] = useState<string | null>(null);

  function handleRowClick(storeName: string) {
    setExpandedStore((prev) => (prev === storeName ? null : storeName));
  }

  return (
    <Card>
      <div className="flex items-center gap-3 mb-3 flex-wrap">
        <h2 className="text-sm font-medium text-[var(--text)]">Datastores</h2>
        {datastores.length > 0 && (
          <span className="inline-flex items-center rounded-md border border-[var(--line)] bg-[var(--surface)] px-1.5 py-0.5 text-xs text-[var(--muted)]">
            {datastores.length}
          </span>
        )}
      </div>

      {datastores.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">No datastores available.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-1.5 px-2 text-left text-[var(--muted)] font-medium">Name</th>
                <th className="py-1.5 px-2 text-left text-[var(--muted)] font-medium">Status</th>
                <th className="py-1.5 px-2 text-left text-[var(--muted)] font-medium">Usage</th>
                <th className="py-1.5 px-2 text-left text-[var(--muted)] font-medium">Groups</th>
                <th className="py-1.5 px-2 text-left text-[var(--muted)] font-medium">Snapshots</th>
                <th className="py-1.5 px-2 text-left text-[var(--muted)] font-medium">Last Backup</th>
                <th className="py-1.5 px-2 text-left text-[var(--muted)] font-medium">Maintenance</th>
              </tr>
            </thead>
            <tbody>
              {datastores.map((store) => {
                const isExpanded = expandedStore === store.store;
                const threshold = usageThreshold(store.usage_percent);
                const lastBackupEpoch = store.last_backup_at
                  ? new Date(store.last_backup_at).getTime() / 1000
                  : undefined;
                const stalenessColor =
                  !lastBackupEpoch || lastBackupEpoch <= 0
                    ? "var(--bad)"
                    : (Date.now() - lastBackupEpoch * 1000) / 3_600_000 > 72
                    ? "var(--bad)"
                    : (Date.now() - lastBackupEpoch * 1000) / 3_600_000 > 24
                    ? "var(--warn)"
                    : "var(--ok)";

                return (
                  <Fragment key={store.store}>
                    <tr
                      className={`border-b border-[var(--line)] border-opacity-30 cursor-pointer transition-colors duration-[var(--dur-fast)] hover:bg-[var(--hover)] ${
                        isExpanded ? "bg-[var(--hover)]" : ""
                      }`}
                      onClick={() => handleRowClick(store.store)}
                    >
                      <td className="py-2 px-2 text-[var(--text)] font-medium">
                        <span className="flex items-center gap-1.5">
                          <span
                            className="text-[var(--muted)] text-[10px] select-none"
                            aria-hidden="true"
                          >
                            {isExpanded ? "\u25BE" : "\u25B8"}
                          </span>
                          {store.store}
                        </span>
                      </td>
                      <td className="py-2 px-2">
                        <Badge status={pbsStatusBadge(store.status)} size="sm" />
                      </td>
                      <td className="py-2 px-2 text-[var(--muted)]">
                        <div className="flex items-center gap-2">
                          <div className="h-1.5 w-20 rounded-full bg-[var(--surface)] shrink-0">
                            <div
                              className="h-full rounded-full transition-[width,background-color] duration-[var(--dur-fast)]"
                              style={{
                                width: `${Math.min(100, store.usage_percent ?? 0)}%`,
                                backgroundColor:
                                  threshold === "bad"
                                    ? "var(--bad)"
                                    : threshold === "warn"
                                    ? "var(--warn)"
                                    : "var(--ok)",
                              }}
                            />
                          </div>
                          <span>
                            {typeof store.usage_percent === "number"
                              ? `${store.usage_percent.toFixed(1)}%`
                              : "n/a"}{" "}
                            ({formatBytes(store.used_bytes)} / {formatBytes(store.total_bytes)})
                          </span>
                        </div>
                      </td>
                      <td className="py-2 px-2 text-[var(--muted)]">{store.group_count ?? 0}</td>
                      <td className="py-2 px-2 text-[var(--muted)]">{store.snapshot_count ?? 0}</td>
                      <td className="py-2 px-2" style={{ color: stalenessColor }}>
                        {store.last_backup_at ? formatRelativeTime(store.last_backup_at) : "never"}
                      </td>
                      <td className="py-2 px-2">
                        {store.maintenance_mode ? (
                          <Badge status="pending" size="sm" />
                        ) : (
                          <span className="text-[var(--muted)]">&mdash;</span>
                        )}
                      </td>
                    </tr>
                    {isExpanded && (
                      <tr>
                        <td colSpan={7} className="p-0 border-b border-[var(--line)] border-opacity-30">
                          <PBSDatastoreDrilldown store={store} assetId={assetId} />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}
