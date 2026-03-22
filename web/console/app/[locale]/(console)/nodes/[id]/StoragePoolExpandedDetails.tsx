"use client";

import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import type { StorageInsightEvent, StorageRow } from "./storageOperationsModel";
import { severityBadgeStatus, taskStatusBadge } from "./storageOperationsUtils";

type StoragePoolExpandedDetailsProps = {
  row: StorageRow;
  rowPoolEvents: StorageInsightEvent[];
  expandedTaskRef: string | null;
  taskLogLoadingRef: string | null;
  taskLogs: Record<string, string>;
  onOpenTaskLog: (node: string, upid: string) => Promise<void> | void;
  onRunPoolBackup: (poolName: string, kind: "vm" | "ct", vmid: number) => Promise<void> | void;
  proxmoxActionRunning: boolean;
  canRunPoolBackup: boolean;
  onOpenWorkloads?: () => void;
};

export function StoragePoolExpandedDetails({
  row,
  rowPoolEvents,
  expandedTaskRef,
  taskLogLoadingRef,
  taskLogs,
  onOpenTaskLog,
  onRunPoolBackup,
  proxmoxActionRunning,
  canRunPoolBackup,
  onOpenWorkloads,
}: StoragePoolExpandedDetailsProps) {
  const latestTaskEvent = rowPoolEvents.find(
    (event) => (event.node?.trim() ?? "") !== "" && (event.upid?.trim() ?? "") !== "",
  );
  const latestTaskRef = latestTaskEvent?.node && latestTaskEvent?.upid
    ? `${latestTaskEvent.node}/${latestTaskEvent.upid}`
    : null;

  return (
    <tr className="border-b border-[var(--line)] border-opacity-30">
      <td colSpan={14} className="py-3 pr-3">
        <div className="grid gap-3 md:grid-cols-2">
          <div className="space-y-2">
            <p className="text-[10px] uppercase tracking-wider text-[var(--muted)]">Quick Actions</p>
            <div className="flex flex-wrap gap-2">
              {row.vmIDs.slice(0, 3).map((vmid) => (
                <Button
                  key={`vm-${row.key}-${vmid}`}
                  size="sm"
                  onClick={() => void onRunPoolBackup(row.poolName, "vm", vmid)}
                  disabled={proxmoxActionRunning || !canRunPoolBackup}
                >
                  Backup VM {vmid}
                </Button>
              ))}
              {row.ctIDs.slice(0, 3).map((vmid) => (
                <Button
                  key={`ct-${row.key}-${vmid}`}
                  size="sm"
                  onClick={() => void onRunPoolBackup(row.poolName, "ct", vmid)}
                  disabled={proxmoxActionRunning || !canRunPoolBackup}
                >
                  Backup CT {vmid}
                </Button>
              ))}
              {latestTaskEvent?.node && latestTaskEvent?.upid ? (
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => void onOpenTaskLog(latestTaskEvent.node!, latestTaskEvent.upid!)}
                >
                  {expandedTaskRef === latestTaskRef ? "Hide Task Log" : "Open Task Log"}
                </Button>
              ) : null}
              {onOpenWorkloads ? (
                <Button size="sm" variant="secondary" onClick={onOpenWorkloads}>
                  Open Workloads
                </Button>
              ) : null}
            </div>
            {row.vmIDs.length === 0 && row.ctIDs.length === 0 ? (
              <p className="text-xs text-[var(--muted)]">No dependent workloads discovered for this pool yet.</p>
            ) : null}
            {row.typeLabel === "zfspool" ? (
              <p className="text-[10px] text-[var(--muted)]">
                Direct ZFS scrub trigger is not exposed in Proxmox API; use backup/task actions here and run scrub from host shell when needed.
              </p>
            ) : null}
            {latestTaskRef && expandedTaskRef === latestTaskRef ? (
              <pre className="mt-2 text-xs text-[var(--muted)] bg-[var(--surface)] rounded p-2 max-h-48 overflow-auto whitespace-pre-wrap">
                {taskLogLoadingRef === latestTaskRef ? "Loading..." : (taskLogs[latestTaskRef] ?? "")}
              </pre>
            ) : null}
          </div>

          <div className="space-y-2">
            <p className="text-[10px] uppercase tracking-wider text-[var(--muted)]">Recent Pool Events</p>
            {rowPoolEvents.length === 0 ? (
              <p className="text-xs text-[var(--muted)]">No storage task events mapped to this pool in the last 24 hours.</p>
            ) : (
              <ul className="divide-y divide-[var(--line)]">
                {rowPoolEvents.slice(0, 4).map((event, index) => (
                  <li
                    key={`${event.timestamp ?? "event"}-${event.upid ?? event.message ?? index}-${index}`}
                    className="py-2 flex items-start justify-between gap-3"
                  >
                    <div>
                      <p className="text-xs text-[var(--text)]">{event.message}</p>
                      <p className="text-[10px] text-[var(--muted)]">
                        {event.timestamp ? new Date(event.timestamp).toLocaleString() : "n/a"}
                      </p>
                    </div>
                    <div className="flex items-center gap-2">
                      <Badge status={severityBadgeStatus(event.severity)} size="sm" />
                      {event.task_status || event.exit_status ? (
                        <Badge status={taskStatusBadge(event.task_status, event.exit_status)} size="sm" />
                      ) : null}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </td>
    </tr>
  );
}
