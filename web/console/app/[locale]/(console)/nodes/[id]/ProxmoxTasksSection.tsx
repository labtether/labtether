"use client";

import { useState } from "react";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import type { ProxmoxTask } from "./nodeDetailTypes";
import { formatProxmoxEpoch, taskStatusBadge } from "./proxmoxFormatters";

type ProxmoxTasksSectionProps = {
  tasks: ProxmoxTask[];
  proxmoxCollectorID: string;
  onRetry: () => void;
};

export function ProxmoxTasksSection({
  tasks,
  proxmoxCollectorID,
  onRetry,
}: ProxmoxTasksSectionProps) {
  const [expandedTaskUpid, setExpandedTaskUpid] = useState<string | null>(null);
  const [taskLogContent, setTaskLogContent] = useState<string | null>(null);
  const [taskLogLoading, setTaskLogLoading] = useState(false);

  return (
    <div className="space-y-2">
      <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">Recent Tasks</p>
      {tasks.length > 0 ? (
        <ul className="divide-y divide-[var(--line)]">
          {tasks.slice(0, 30).map((task, idx) => (
            <li key={`${task.upid ?? "task"}-${idx}`} className="py-2.5">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <span className="text-sm font-medium text-[var(--text)]">{task.type || "task"}</span>
                  <code className="block text-xs text-[var(--muted)]">{task.id || task.upid || "unknown"}</code>
                </div>
                <div className="flex items-center gap-2">
                  <Badge status={taskStatusBadge(task)} />
                  <span className="text-xs text-[var(--muted)]">{formatProxmoxEpoch(task.starttime)}</span>
                  {task.upid && task.node ? (
                    <button
                      className="text-xs text-[var(--accent)] hover:underline"
                      onClick={async () => {
                        const taskUpid = task.upid;
                        const taskNode = task.node;
                        if (!taskUpid || !taskNode) {
                          return;
                        }
                        if (expandedTaskUpid === taskUpid) {
                          setExpandedTaskUpid(null);
                          setTaskLogContent(null);
                          return;
                        }
                        setExpandedTaskUpid(taskUpid);
                        setTaskLogLoading(true);
                        setTaskLogContent(null);
                        try {
                          const collectorQuery = proxmoxCollectorID !== ""
                            ? `?collector_id=${encodeURIComponent(proxmoxCollectorID)}`
                            : "";
                          const response = await fetch(`/api/proxmox/tasks/${encodeURIComponent(taskNode)}/${encodeURIComponent(taskUpid)}/log${collectorQuery}`, { signal: AbortSignal.timeout(15_000) });
                          if (response.ok) {
                            const data = (await response.json()) as { log?: string };
                            setTaskLogContent(data.log ?? "No log content.");
                          } else {
                            setTaskLogContent("Failed to load log.");
                          }
                        } catch {
                          setTaskLogContent("Failed to load log.");
                        } finally {
                          setTaskLogLoading(false);
                        }
                      }}
                    >
                      {expandedTaskUpid === task.upid ? "Hide" : "Log"}
                    </button>
                  ) : null}
                  {task.upid && task.node && taskStatusBadge(task) === "pending" ? (
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={async () => {
                        const taskUpid = task.upid;
                        const taskNode = task.node;
                        if (!taskUpid || !taskNode) {
                          return;
                        }
                        if (!confirm("Cancel this running task?")) {
                          return;
                        }
                        try {
                          const collectorQuery = proxmoxCollectorID !== ""
                            ? `?collector_id=${encodeURIComponent(proxmoxCollectorID)}`
                            : "";
                          await fetch(`/api/proxmox/tasks/${encodeURIComponent(taskNode)}/${encodeURIComponent(taskUpid)}/stop${collectorQuery}`, { method: "POST" });
                          onRetry();
                        } catch {
                          // Swallow to preserve existing page behavior.
                        }
                      }}
                    >
                      Cancel
                    </Button>
                  ) : null}
                </div>
              </div>
              {expandedTaskUpid === task.upid ? (
                <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap rounded bg-[var(--surface)] p-2 text-xs text-[var(--muted)]">
                  {taskLogLoading ? "Loading..." : taskLogContent ?? ""}
                </pre>
              ) : null}
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-xs text-[var(--muted)]">No matching task history returned.</p>
      )}
    </div>
  );
}
