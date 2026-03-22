"use client";

import { Badge } from "../../../../components/ui/Badge";
import { Card } from "../../../../components/ui/Card";
import type { ActionRun } from "../../../../console/models";

type NodeActionsTabCardProps = {
  actionsLoading: boolean;
  actionRuns: ActionRun[];
  expandedActionId: string | null;
  onExpandedActionChange: (nextID: string | null) => void;
};

export function NodeActionsTabCard({
  actionsLoading,
  actionRuns,
  expandedActionId,
  onExpandedActionChange,
}: NodeActionsTabCardProps) {
  return (
    <Card className="mb-4">
      <h2 className="text-sm font-medium text-[var(--text)] mb-3">Device Actions</h2>
      {actionsLoading ? (
        <p className="text-sm text-[var(--muted)]">Loading actions...</p>
      ) : actionRuns.length > 0 ? (
        <ul className="divide-y divide-[var(--line)]">
          {actionRuns.map((run) => {
            const borderColor = run.status === "succeeded"
              ? "border-l-[var(--ok)]"
              : run.status === "failed"
                ? "border-l-[var(--bad)]"
                : "border-l-[var(--warn)]";
            const isExpanded = expandedActionId === run.id;
            const hasOutput = Boolean(run.output || run.error);
            return (
              <li key={run.id} className={`py-2.5 pl-2 border-l-2 ${borderColor}`}>
                <div
                  className={`flex items-center justify-between gap-3 ${hasOutput ? "cursor-pointer" : ""}`}
                  onClick={() => {
                    if (!hasOutput) {
                      return;
                    }
                    onExpandedActionChange(isExpanded ? null : run.id);
                  }}
                >
                  <div>
                    <span className="text-sm font-medium text-[var(--text)]">{run.type}{run.action_id ? ` / ${run.action_id}` : ""}</span>
                    <code className="block text-xs text-[var(--muted)]">{run.command ?? run.connector_id ?? "\u2014"}</code>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge status={run.status === "succeeded" ? "ok" : run.status === "failed" ? "bad" : "pending"} size="sm" />
                    <span className="text-xs text-[var(--muted)]">{run.created_at ? new Date(run.created_at).toLocaleString() : "n/a"}</span>
                  </div>
                </div>
                {isExpanded && (run.output || run.error) ? (
                  <pre className="mt-2 text-xs text-[var(--muted)] bg-[var(--surface)] rounded p-2 max-h-48 overflow-auto whitespace-pre-wrap">
                    {run.error ? `Error: ${run.error}\n` : ""}
                    {run.output ?? ""}
                  </pre>
                ) : null}
              </li>
            );
          })}
        </ul>
      ) : (
        <div className="flex flex-col items-center justify-center py-12 gap-2">
          <p className="text-sm font-medium text-[var(--text)]">Nothing run yet</p>
          <p className="text-xs text-[var(--muted)] text-center max-w-sm">No actions have been run on this device yet.</p>
        </div>
      )}
    </Card>
  );
}
