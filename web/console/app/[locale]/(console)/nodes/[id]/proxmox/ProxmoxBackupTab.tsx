"use client";

import { Card } from "../../../../../components/ui/Card";
import type { ProxmoxDetails } from "../nodeDetailTypes";

type Props = {
  proxmoxDetails: ProxmoxDetails;
};

export function ProxmoxBackupTab({ proxmoxDetails }: Props) {
  const schedules = proxmoxDetails.backup_schedules ?? [];

  return (
    <div className="space-y-4">
      <Card>
        <h2 className="mb-3 text-sm font-medium text-[var(--text)]">
          Backup Schedules{schedules.length > 0 ? ` (${schedules.length})` : ""}
        </h2>
        {schedules.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">ID</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Schedule</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Storage</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Mode</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Compress</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">VMs</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Max Files</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Node</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Enabled</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Comment</th>
                </tr>
              </thead>
              <tbody>
                {schedules.map((sched, idx) => (
                  <tr key={sched.id || idx} className="border-b border-[var(--line)] border-opacity-30">
                    <td className="px-2 py-1 font-mono text-[10px] text-[var(--muted)]">{sched.id || "-"}</td>
                    <td className="px-2 py-1 text-[var(--text)]">{sched.schedule || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{sched.storage || "-"}</td>
                    <td className="px-2 py-1">
                      <span className="rounded bg-[var(--hover)] px-1.5 py-0.5 text-[10px] text-[var(--text)]">
                        {sched.mode || "-"}
                      </span>
                    </td>
                    <td className="px-2 py-1 text-[var(--muted)]">{sched.compress || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{sched.vmid || "all"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">
                      {sched.maxfiles != null ? String(sched.maxfiles) : "-"}
                    </td>
                    <td className="px-2 py-1 text-[var(--muted)]">{sched.node || "all"}</td>
                    <td className="px-2 py-1">
                      <span className={sched.enabled ? "text-[var(--ok)]" : "text-[var(--bad)]"}>
                        {sched.enabled ? "Yes" : "No"}
                      </span>
                    </td>
                    <td className="max-w-[200px] truncate px-2 py-1 text-[var(--muted)]">
                      {sched.comment || ""}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="text-xs text-[var(--muted)]">No backup schedules returned.</p>
        )}
      </Card>
    </div>
  );
}
