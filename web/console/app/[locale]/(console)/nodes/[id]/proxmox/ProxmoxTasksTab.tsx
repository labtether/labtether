"use client";

import { useState } from "react";
import { Card } from "../../../../../components/ui/Card";
import type { ProxmoxTask } from "../nodeDetailTypes";
import { ProxmoxTasksSection } from "../ProxmoxTasksSection";

type Props = {
  tasks: ProxmoxTask[];
  proxmoxCollectorID: string;
  onRetry: () => void;
};

const STATUS_OPTIONS = ["all", "running", "ok", "error"] as const;
type StatusFilter = (typeof STATUS_OPTIONS)[number];

export function ProxmoxTasksTab({ tasks, proxmoxCollectorID, onRetry }: Props) {
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [typeFilter, setTypeFilter] = useState("");

  const uniqueTypes = Array.from(
    new Set(tasks.map((t) => t.type ?? "").filter(Boolean)),
  ).sort();

  const filtered = tasks.filter((t) => {
    if (statusFilter !== "all") {
      const status = (t.status ?? "").toLowerCase();
      const exit = (t.exitstatus ?? "").toLowerCase();
      if (statusFilter === "running" && status !== "running") return false;
      if (statusFilter === "ok" && (status === "running" || (exit !== "" && exit !== "ok") || status === "error")) return false;
      if (statusFilter === "error" && status !== "error" && (exit === "" || exit === "ok")) return false;
    }
    if (typeFilter && t.type !== typeFilter) return false;
    return true;
  });

  return (
    <Card>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">Tasks</h2>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <select
            className="rounded border border-[var(--line)] bg-[var(--surface)] px-2 py-1 text-xs text-[var(--text)]"
            value={statusFilter}
            onChange={(e) => { setStatusFilter(e.target.value as StatusFilter); }}
          >
            {STATUS_OPTIONS.map((opt) => (
              <option key={opt} value={opt}>{opt === "all" ? "All statuses" : opt}</option>
            ))}
          </select>
          {uniqueTypes.length > 0 ? (
            <select
              className="rounded border border-[var(--line)] bg-[var(--surface)] px-2 py-1 text-xs text-[var(--text)]"
              value={typeFilter}
              onChange={(e) => { setTypeFilter(e.target.value); }}
            >
              <option value="">All types</option>
              {uniqueTypes.map((t) => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
          ) : null}
        </div>
      </div>
      <ProxmoxTasksSection
        tasks={filtered}
        proxmoxCollectorID={proxmoxCollectorID}
        onRetry={onRetry}
      />
    </Card>
  );
}
