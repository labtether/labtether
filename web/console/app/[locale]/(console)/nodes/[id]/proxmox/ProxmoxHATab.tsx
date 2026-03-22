"use client";

import { Card } from "../../../../../components/ui/Card";
import type { ProxmoxDetails } from "../nodeDetailTypes";

type Props = {
  proxmoxDetails: ProxmoxDetails;
};

export function ProxmoxHATab({ proxmoxDetails }: Props) {
  const match = proxmoxDetails.ha?.match;
  const resources = proxmoxDetails.ha?.resources ?? [];

  return (
    <div className="space-y-4">
      <Card>
        <h2 className="mb-3 text-sm font-medium text-[var(--text)]">HA Resource</h2>
        {match ? (
          <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5">
            <div>
              <dt className="text-xs text-[var(--muted)]">Resource</dt>
              <dd className="text-xs text-[var(--text)]">{match.sid || "unknown"}</dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">State</dt>
              <dd className="text-xs text-[var(--text)]">{match.state || "unknown"}</dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">Status</dt>
              <dd className="text-xs text-[var(--text)]">{match.status || "unknown"}</dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">Group</dt>
              <dd className="text-xs text-[var(--text)]">{match.group || "default"}</dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">Node</dt>
              <dd className="text-xs text-[var(--text)]">{match.node || "-"}</dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">Max Restart</dt>
              <dd className="text-xs text-[var(--text)]">
                {match.max_restart != null ? String(match.max_restart) : "-"}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">Max Relocate</dt>
              <dd className="text-xs text-[var(--text)]">
                {match.max_relocate != null ? String(match.max_relocate) : "-"}
              </dd>
            </div>
            {match.comment ? (
              <div className="col-span-2">
                <dt className="text-xs text-[var(--muted)]">Comment</dt>
                <dd className="text-xs text-[var(--text)]">{match.comment}</dd>
              </div>
            ) : null}
          </dl>
        ) : (
          <p className="text-xs text-[var(--muted)]">No HA resource matched this asset.</p>
        )}
      </Card>

      {resources.length > 0 ? (
        <Card>
          <h2 className="mb-3 text-sm font-medium text-[var(--text)]">
            All HA Resources ({resources.length})
          </h2>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">SID</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">State</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Status</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Group</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Node</th>
                </tr>
              </thead>
              <tbody>
                {resources.map((r, idx) => (
                  <tr key={r.sid ?? idx} className="border-b border-[var(--line)] border-opacity-30">
                    <td className="px-2 py-1 font-mono text-[10px] text-[var(--text)]">{r.sid || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{r.state || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{r.status || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{r.group || "default"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{r.node || "-"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      ) : null}
    </div>
  );
}
