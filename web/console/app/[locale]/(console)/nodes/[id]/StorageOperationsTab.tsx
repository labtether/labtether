"use client";

import { useState } from "react";

import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import type { Asset } from "../../../../console/models";
import type { ProxmoxStorageDetails } from "./storageOperationsModel";
import {
  recommendationStatus,
  severityBadgeStatus,
  taskStatusBadge,
} from "./storageOperationsUtils";
import { StorageOperationsTableCard } from "./StorageOperationsTableCard";
import { useStorageOperationsActions } from "./useStorageOperationsActions";
import { useStorageOperationsData } from "./useStorageOperationsData";

type StorageOperationsTabProps = {
  hostAsset: Asset;
  proxmoxDetails: ProxmoxStorageDetails | null;
  proxmoxLoading: boolean;
  proxmoxError: string | null;
  onRetry: () => void;
  onRunProxmoxAction?: (
    actionID: string,
    target: string,
    params?: Record<string, string>,
  ) => Promise<void> | void;
  proxmoxActionRunning?: boolean;
  proxmoxActionMessage?: string | null;
  proxmoxActionError?: string | null;
  onOpenWorkloads?: () => void;
};

export function StorageOperationsTab({
  hostAsset,
  proxmoxDetails,
  proxmoxLoading,
  proxmoxError,
  onRetry,
  onRunProxmoxAction,
  proxmoxActionRunning = false,
  proxmoxActionMessage = null,
  proxmoxActionError = null,
  onOpenWorkloads,
}: StorageOperationsTabProps) {
  const [expandedRowKey, setExpandedRowKey] = useState<string | null>(null);

  const {
    riskFilter,
    setRiskFilter,
    insights,
    insightsLoading,
    insightsError,
    fetchInsights,
    storageNode,
    proxmoxCollectorID,
    proxmoxStaleInfo,
    rows,
    rowByKey,
    timelineEvents,
    poolEvents,
    recommendations,
    filteredRows,
    riskChips,
  } = useStorageOperationsData({
    hostAsset,
    proxmoxDetails,
  });

  const {
    expandedTaskRef,
    taskLogLoadingRef,
    taskLogs,
    taskLogError,
    localActionError,
    openTaskLog,
    runPoolBackup,
  } = useStorageOperationsActions({
    fetchInsights,
    onRunProxmoxAction,
    proxmoxCollectorID,
    storageNode,
  });

  return (
    <div className="space-y-4">
      <Card>
        <div className="flex items-start justify-between gap-3 mb-3">
          <div>
            <h2 className="text-sm font-medium text-[var(--text)]">Storage Risk Strip</h2>
            <p className="text-xs text-[var(--muted)]">Filter pools by active risk signals.</p>
          </div>
          <div className="text-right">
            <p className="text-xs text-[var(--muted)]">Proxmox details: {proxmoxStaleInfo.label}</p>
            <p className="text-xs text-[var(--muted)]">Insights window: {insights?.window ?? "7d"}</p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {riskChips.map((chip) => (
            <button
              key={chip.key}
              className={`px-2.5 py-1.5 rounded-lg border text-xs transition-colors duration-150 ${
                riskFilter === chip.key
                  ? "border-[var(--accent)] text-[var(--text)] bg-[var(--hover)]"
                  : "border-[var(--line)] text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)]"
              }`}
              onClick={() => setRiskFilter(chip.key)}
            >
              {chip.label}: {chip.value}
            </button>
          ))}
        </div>
      </Card>

      <StorageOperationsTableCard
        proxmoxLoading={proxmoxLoading}
        insightsLoading={insightsLoading}
        rows={rows}
        filteredRows={filteredRows}
        poolEvents={poolEvents}
        expandedRowKey={expandedRowKey}
        onExpandedRowChange={setExpandedRowKey}
        proxmoxError={proxmoxError}
        insightsError={insightsError}
        proxmoxActionError={proxmoxActionError}
        localActionError={localActionError}
        taskLogError={taskLogError}
        proxmoxActionMessage={proxmoxActionMessage}
        onRefresh={() => {
          onRetry();
          void fetchInsights();
        }}
        onOpenTaskLog={openTaskLog}
        expandedTaskRef={expandedTaskRef}
        taskLogLoadingRef={taskLogLoadingRef}
        taskLogs={taskLogs}
        onRunPoolBackup={runPoolBackup}
        proxmoxActionRunning={proxmoxActionRunning}
        canRunPoolBackup={!!onRunProxmoxAction}
        onOpenWorkloads={onOpenWorkloads}
      />

      <Card>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-[var(--text)]">Storage Timeline (24h)</h2>
          <span className="text-xs text-[var(--muted)]">{timelineEvents.length} events</span>
        </div>
        {timelineEvents.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No storage-related task events in the last 24 hours.</p>
        ) : (
          <ul className="divide-y divide-[var(--line)]">
            {timelineEvents.slice(0, 20).map((event, index) => {
              const taskRef = event.node && event.upid ? `${event.node}/${event.upid}` : null;
              const isExpanded = taskRef != null && expandedTaskRef === taskRef;

              return (
                <li
                  key={`${event.timestamp ?? "event"}-${event.upid ?? event.message ?? index}-${index}`}
                  className="py-2.5"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="text-sm text-[var(--text)]">{event.message}</p>
                      <p className="text-[10px] text-[var(--muted)] mt-0.5">
                        {event.pool ? `${event.pool} • ` : ""}
                        {event.timestamp ? new Date(event.timestamp).toLocaleString() : "n/a"}
                      </p>
                    </div>
                    <div className="flex items-center gap-2">
                      <Badge status={severityBadgeStatus(event.severity)} size="sm" />
                      {event.task_status || event.exit_status ? (
                        <Badge status={taskStatusBadge(event.task_status, event.exit_status)} size="sm" />
                      ) : null}
                      {event.node && event.upid ? (
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => void openTaskLog(event.node!, event.upid!)}
                        >
                          {isExpanded ? "Hide Log" : "Log"}
                        </Button>
                      ) : null}
                    </div>
                  </div>
                  {taskRef && isExpanded ? (
                    <pre className="mt-2 text-xs text-[var(--muted)] bg-[var(--surface)] rounded p-2 max-h-48 overflow-auto whitespace-pre-wrap">
                      {taskLogLoadingRef === taskRef ? "Loading..." : (taskLogs[taskRef] ?? "")}
                    </pre>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
      </Card>

      <Card>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-[var(--text)]">Recommended Next Actions</h2>
          <span className="text-xs text-[var(--muted)]">{recommendations.length} items</span>
        </div>
        {recommendations.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No immediate storage actions. Continue monitoring capacity and health.</p>
        ) : (
          <ul className="divide-y divide-[var(--line)]">
            {recommendations.slice(0, 6).map((item) => {
              const row = rowByKey.get(item.rowKey);
              return (
                <li key={item.key} className="py-2.5 flex items-start justify-between gap-3">
                  <div>
                    <p className="text-sm font-medium text-[var(--text)]">{item.poolName}</p>
                    <p className="text-xs text-[var(--muted)]">{item.message}</p>
                    <p className="text-[10px] text-[var(--muted)] uppercase tracking-wider mt-0.5">confidence: {item.confidence}</p>
                    {row ? (
                      <button
                        className="text-xs text-[var(--accent)] hover:underline mt-1"
                        onClick={() => {
                          setRiskFilter("all");
                          setExpandedRowKey(row.key);
                        }}
                      >
                        Open pool details
                      </button>
                    ) : null}
                  </div>
                  <div className="flex items-center gap-2">
                    {item.backupTarget ? (
                      <Button
                        size="sm"
                        onClick={() => void runPoolBackup(item.poolName, item.backupTarget!.kind, item.backupTarget!.vmid)}
                        disabled={proxmoxActionRunning || !onRunProxmoxAction}
                      >
                        Backup {item.backupTarget.kind.toUpperCase()} {item.backupTarget.vmid}
                      </Button>
                    ) : null}
                    <Badge status={recommendationStatus(item.severity)} size="sm" />
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </Card>
    </div>
  );
}
