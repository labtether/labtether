"use client";

import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import type { PBSTask, PBSTaskLogLine, PBSTaskStatus } from "./pbsTabModel";

type PBSTaskDetailsCardProps = {
  selectedTask: PBSTask;
  taskStatus: PBSTaskStatus | null;
  taskLog: PBSTaskLogLine[];
  taskLoading: boolean;
  stoppingTask: boolean;
  error: string | null;
  nodeFallback: string;
  onRefresh: () => void;
  onStopTask: () => void;
};

export function PBSTaskDetailsCard({
  selectedTask,
  taskStatus,
  taskLog,
  taskLoading,
  stoppingTask,
  error,
  nodeFallback,
  onRefresh,
  onStopTask,
}: PBSTaskDetailsCardProps) {
  return (
    <Card>
      <div className="flex items-center justify-between mb-3 gap-3 flex-wrap">
        <h2 className="text-sm font-medium text-[var(--text)]">Task Details</h2>
        <div className="flex items-center gap-2">
          <Button size="sm" onClick={onRefresh} disabled={taskLoading || stoppingTask}>
            {taskLoading ? "Refreshing..." : "Refresh"}
          </Button>
          <Button size="sm" onClick={onStopTask} disabled={taskLoading || stoppingTask}>
            {stoppingTask ? "Stopping..." : "Stop Task"}
          </Button>
        </div>
      </div>
      {error ? <p className="text-xs text-[var(--bad)] mb-3">{error}</p> : null}
      <div className="flex flex-wrap gap-2 mb-3">
        <Chip label="Node" value={selectedTask.node || nodeFallback} />
        <Chip label="Worker" value={selectedTask.worker_type || "task"} />
        <Chip label="Status" value={taskStatus?.status || selectedTask.status || "unknown"} />
        <Chip label="Exit" value={taskStatus?.exitstatus || "n/a"} />
      </div>
      {taskLog.length > 0 ? (
        <pre className="max-h-80 overflow-auto rounded-md border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-[11px] text-[var(--text)]">
          {taskLog.map((line) => `${String(line.n).padStart(4, " ")}  ${line.t}`).join("\n")}
        </pre>
      ) : (
        <p className="text-xs text-[var(--muted)]">No task log lines returned.</p>
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
