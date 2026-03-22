"use client";

import { Fragment } from "react";

import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import type { StorageInsightEvent, StorageRow } from "./storageOperationsModel";
import { StorageOperationsPoolRow } from "./StorageOperationsPoolRow";
import { StoragePoolExpandedDetails } from "./StoragePoolExpandedDetails";

type StorageOperationsTableCardProps = {
  proxmoxLoading: boolean;
  insightsLoading: boolean;
  rows: StorageRow[];
  filteredRows: StorageRow[];
  poolEvents: Map<string, StorageInsightEvent[]>;
  expandedRowKey: string | null;
  onExpandedRowChange: (nextKey: string | null) => void;
  proxmoxError: string | null;
  insightsError: string | null;
  proxmoxActionError: string | null;
  localActionError: string | null;
  taskLogError: string | null;
  proxmoxActionMessage: string | null;
  onRefresh: () => void;
  onOpenTaskLog: (node: string, upid: string) => Promise<void> | void;
  expandedTaskRef: string | null;
  taskLogLoadingRef: string | null;
  taskLogs: Record<string, string>;
  onRunPoolBackup: (poolName: string, kind: "vm" | "ct", vmid: number) => Promise<void> | void;
  proxmoxActionRunning: boolean;
  canRunPoolBackup: boolean;
  onOpenWorkloads?: () => void;
};

export function StorageOperationsTableCard({
  proxmoxLoading,
  insightsLoading,
  rows,
  filteredRows,
  poolEvents,
  expandedRowKey,
  onExpandedRowChange,
  proxmoxError,
  insightsError,
  proxmoxActionError,
  localActionError,
  taskLogError,
  proxmoxActionMessage,
  onRefresh,
  onOpenTaskLog,
  expandedTaskRef,
  taskLogLoadingRef,
  taskLogs,
  onRunPoolBackup,
  proxmoxActionRunning,
  canRunPoolBackup,
  onOpenWorkloads,
}: StorageOperationsTableCardProps) {
  return (
    <Card>
      <div className="flex items-start justify-between gap-3 mb-3">
        <div>
          <h2 className="text-sm font-medium text-[var(--text)]">Storage Operations</h2>
          <p className="text-xs text-[var(--muted)]">Risk-ordered table with forecast and workload impact.</p>
        </div>
        <div className="text-right space-y-1">
          {(proxmoxError || insightsError || proxmoxActionError || localActionError || taskLogError) ? (
            <p className="text-xs text-[var(--bad)]">{proxmoxError ?? insightsError ?? proxmoxActionError ?? localActionError ?? taskLogError}</p>
          ) : proxmoxActionMessage ? (
            <p className="text-xs text-[var(--ok)]">{proxmoxActionMessage}</p>
          ) : null}
          <Button size="sm" onClick={onRefresh}>Refresh</Button>
        </div>
      </div>

      {(proxmoxLoading || insightsLoading) && rows.length === 0 ? (
        <p className="text-sm text-[var(--muted)]">Loading storage details...</p>
      ) : filteredRows.length === 0 ? (
        <p className="text-sm text-[var(--muted)] py-8 text-center">
          {rows.length === 0 ? "No storage pools discovered for this host." : "No pools match this filter."}
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full text-xs">
            <thead>
              <tr className="text-left border-b border-[var(--line)]">
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Pool</th>
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Health</th>
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Used</th>
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Free</th>
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Growth (7d)</th>
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Days to 80%</th>
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Days to Full</th>
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Frag</th>
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Dedup</th>
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Snapshots</th>
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Workloads</th>
                <th className="py-2 pr-3 text-[var(--muted)] font-medium">Signal</th>
                <th className="py-2 text-[var(--muted)] font-medium">Status</th>
                <th className="py-2 pl-3 text-[var(--muted)] font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filteredRows.map((row) => {
                const isExpanded = expandedRowKey === row.key;
                const rowPoolEvents = poolEvents.get(row.key) ?? [];

                return (
                  <Fragment key={row.key}>
                    <StorageOperationsPoolRow
                      row={row}
                      isExpanded={isExpanded}
                      onToggleExpanded={() => onExpandedRowChange(isExpanded ? null : row.key)}
                    />
                    {isExpanded ? (
                      <StoragePoolExpandedDetails
                        row={row}
                        rowPoolEvents={rowPoolEvents}
                        expandedTaskRef={expandedTaskRef}
                        taskLogLoadingRef={taskLogLoadingRef}
                        taskLogs={taskLogs}
                        onOpenTaskLog={onOpenTaskLog}
                        onRunPoolBackup={onRunPoolBackup}
                        proxmoxActionRunning={proxmoxActionRunning}
                        canRunPoolBackup={canRunPoolBackup}
                        onOpenWorkloads={onOpenWorkloads}
                      />
                    ) : null}
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
