"use client";

import { Badge } from "../../../../components/ui/Badge";
import { Card } from "../../../../components/ui/Card";
import { formatRelativeEpoch, pbsTaskStatusBadge } from "./pbsTabModel";
import type { PBSTask } from "./pbsTabModel";

type PBSRecentTasksCardProps = {
  tasks: PBSTask[];
  selectedTask: PBSTask | null;
  onSelectTask: (task: PBSTask) => void;
};

function formatDuration(startEpoch: number, endEpoch: number): string {
  const seconds = Math.max(0, endEpoch - startEpoch);
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
}

function taskTypeLabel(workerType: string): string {
  const labels: Record<string, string> = {
    backup: "Backup",
    verificationjob: "Verify",
    verify: "Verify",
    prune: "Prune",
    garbage_collection: "GC",
    gc: "GC",
    sync: "Sync",
    reader: "Restore",
    datastore_verify: "Verify",
  };
  return labels[workerType.toLowerCase()] ?? workerType;
}

export function PBSRecentTasksCard({ tasks, selectedTask, onSelectTask }: PBSRecentTasksCardProps) {
  return (
    <Card>
      <div className="flex items-center justify-between mb-3 gap-3 flex-wrap">
        <h2 className="text-sm font-medium text-[var(--text)]">Recent PBS Tasks</h2>
        <span className="text-xs text-[var(--muted)]">{tasks.length} tasks</span>
      </div>
      {tasks.length > 0 ? (
        <ul className="divide-y divide-[var(--line)]">
          {tasks.slice(0, 30).map((task) => {
            const active = selectedTask?.upid === task.upid;
            return (
              <li key={task.upid} className="py-2">
                <button
                  className={`w-full text-left rounded-md px-2 py-1 transition-colors ${
                    active ? "bg-[var(--hover)]" : "hover:bg-[var(--hover)]"
                  }`}
                  onClick={() => onSelectTask(task)}
                >
                  <div className="flex items-center gap-2">
                    <Badge status={pbsTaskStatusBadge(task.status)} size="sm" />
                    <span className="text-xs font-medium text-[var(--text)]">
                      {taskTypeLabel(task.worker_type || "task")}
                    </span>
                    {task.worker_id ? <span className="text-xs text-[var(--muted)]">{task.worker_id}</span> : null}
                    <span className="ml-auto text-xs text-[var(--muted)]">
                      {task.starttime ? formatRelativeEpoch(task.starttime) : "n/a"}
                      {task.starttime && task.endtime ? (
                        <span className="text-[var(--muted)] ml-1">({formatDuration(task.starttime, task.endtime)})</span>
                      ) : null}
                    </span>
                  </div>
                  <p className="text-[11px] text-[var(--muted)] mt-1 truncate">{task.upid}</p>
                </button>
              </li>
            );
          })}
        </ul>
      ) : (
        <p className="text-xs text-[var(--muted)]">No recent tasks returned by PBS.</p>
      )}
    </Card>
  );
}
