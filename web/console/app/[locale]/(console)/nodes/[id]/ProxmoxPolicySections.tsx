"use client";

import type { ProxmoxDetails } from "./nodeDetailTypes";

type ProxmoxPolicySectionsProps = {
  proxmoxDetails: ProxmoxDetails;
};

export function ProxmoxPolicySections({ proxmoxDetails }: ProxmoxPolicySectionsProps) {
  return (
    <>
      <div className="space-y-2">
        <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">HA</p>
        {proxmoxDetails.ha?.match ? (
          <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5">
            <div>
              <dt className="text-xs text-[var(--muted)]">Resource</dt>
              <dd className="text-xs text-[var(--text)]">{proxmoxDetails.ha.match.sid || "unknown"}</dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">State</dt>
              <dd className="text-xs text-[var(--text)]">{proxmoxDetails.ha.match.state || "unknown"}</dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">Status</dt>
              <dd className="text-xs text-[var(--text)]">{proxmoxDetails.ha.match.status || "unknown"}</dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">Group</dt>
              <dd className="text-xs text-[var(--text)]">{proxmoxDetails.ha.match.group || "default"}</dd>
            </div>
          </dl>
        ) : (
          <p className="text-xs text-[var(--muted)]">No HA resource matched this asset.</p>
        )}
        {proxmoxDetails.ha?.resources && proxmoxDetails.ha.resources.length > 0 ? (
          <p className="text-xs text-[var(--muted)]">Related HA resources: {proxmoxDetails.ha.resources.length}</p>
        ) : null}
      </div>

      {proxmoxDetails.firewall_rules && proxmoxDetails.firewall_rules.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">
            Firewall Rules ({proxmoxDetails.firewall_rules.length})
          </p>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">#</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Type</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Action</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Source</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Dest</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Proto</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Port</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Enabled</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Comment</th>
                </tr>
              </thead>
              <tbody>
                {proxmoxDetails.firewall_rules.map((rule, idx) => (
                  <tr key={idx} className="border-b border-[var(--line)] border-opacity-30">
                    <td className="px-2 py-1 text-[var(--muted)]">{rule.pos}</td>
                    <td className="px-2 py-1">{rule.type}</td>
                    <td className="px-2 py-1">
                      <span className={`px-1.5 py-0.5 rounded text-[10px] ${
                        rule.action === "ACCEPT" ? "bg-[var(--ok-glow)] text-[var(--ok)]" :
                        rule.action === "DROP" ? "bg-[var(--bad-glow)] text-[var(--bad)]" :
                        "bg-[var(--warn-glow)] text-[var(--warn)]"
                      }`}>{rule.action}</span>
                    </td>
                    <td className="px-2 py-1 text-[var(--muted)]">{rule.source || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{rule.dest || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{rule.proto || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{rule.dport || "-"}</td>
                    <td className="px-2 py-1">{rule.enable ? "Yes" : "No"}</td>
                    <td className="max-w-[200px] truncate px-2 py-1 text-[var(--muted)]">{rule.comment || ""}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}

      {proxmoxDetails.backup_schedules && proxmoxDetails.backup_schedules.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">
            Backup Schedules ({proxmoxDetails.backup_schedules.length})
          </p>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Schedule</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Storage</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Mode</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">VMs</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Enabled</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Comment</th>
                </tr>
              </thead>
              <tbody>
                {proxmoxDetails.backup_schedules.map((sched, idx) => (
                  <tr key={sched.id || idx} className="border-b border-[var(--line)] border-opacity-30">
                    <td className="px-2 py-1 text-[var(--text)]">{sched.schedule || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{sched.storage || "-"}</td>
                    <td className="px-2 py-1">
                      <span className="rounded bg-[var(--hover)] px-1.5 py-0.5 text-[10px] text-[var(--text)]">{sched.mode || "-"}</span>
                    </td>
                    <td className="px-2 py-1 text-[var(--muted)]">{sched.vmid || "all"}</td>
                    <td className="px-2 py-1">
                      <span className={sched.enabled ? "text-[var(--ok)]" : "text-[var(--bad)]"}>
                        {sched.enabled ? "Yes" : "No"}
                      </span>
                    </td>
                    <td className="max-w-[200px] truncate px-2 py-1 text-[var(--muted)]">{sched.comment || ""}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
    </>
  );
}
