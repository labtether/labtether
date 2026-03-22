"use client";

import { Card } from "../../../../../components/ui/Card";
import type { ProxmoxDetails } from "../nodeDetailTypes";

type Props = {
  proxmoxDetails: ProxmoxDetails;
};

export function ProxmoxFirewallTab({ proxmoxDetails }: Props) {
  const rules = proxmoxDetails.firewall_rules ?? [];

  return (
    <Card>
      <h2 className="mb-3 text-sm font-medium text-[var(--text)]">
        Firewall Rules{rules.length > 0 ? ` (${rules.length})` : ""}
      </h2>
      {rules.length > 0 ? (
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
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Iface</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Enabled</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Comment</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((rule, idx) => (
                <tr key={idx} className="border-b border-[var(--line)] border-opacity-30">
                  <td className="px-2 py-1 text-[var(--muted)]">{rule.pos}</td>
                  <td className="px-2 py-1 text-[var(--text)]">{rule.type}</td>
                  <td className="px-2 py-1">
                    <span
                      className={`rounded px-1.5 py-0.5 text-[10px] ${
                        rule.action === "ACCEPT"
                          ? "bg-[var(--ok-glow)] text-[var(--ok)]"
                          : rule.action === "DROP"
                            ? "bg-[var(--bad-glow)] text-[var(--bad)]"
                            : "bg-[var(--warn-glow)] text-[var(--warn)]"
                      }`}
                    >
                      {rule.action}
                    </span>
                  </td>
                  <td className="px-2 py-1 text-[var(--muted)]">{rule.source || "-"}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{rule.dest || "-"}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{rule.proto || "-"}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{rule.dport || "-"}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{rule.iface || "-"}</td>
                  <td className="px-2 py-1">
                    <span className={rule.enable ? "text-[var(--ok)]" : "text-[var(--bad)]"}>
                      {rule.enable ? "Yes" : "No"}
                    </span>
                  </td>
                  <td className="max-w-[200px] truncate px-2 py-1 text-[var(--muted)]">
                    {rule.comment || ""}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="text-xs text-[var(--muted)]">No firewall rules returned.</p>
      )}
    </Card>
  );
}
