"use client";

import { Badge } from "../../../../../components/ui/Badge";
import { Card } from "../../../../../components/ui/Card";
import type { NetworkInterface } from "../nodeDetailTypes";

type Props = {
  networkInterfaces: NetworkInterface[];
};

export function ProxmoxNetworkTab({ networkInterfaces }: Props) {
  return (
    <Card>
      <h2 className="mb-3 text-sm font-medium text-[var(--text)]">
        Network Interfaces{networkInterfaces.length > 0 ? ` (${networkInterfaces.length})` : ""}
      </h2>
      {networkInterfaces.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Interface</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Type</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Address / CIDR</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Gateway</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Bridge Ports</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Method</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Active</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Autostart</th>
              </tr>
            </thead>
            <tbody>
              {networkInterfaces.map((iface, idx) => (
                <tr key={`${iface.iface ?? "iface"}-${idx}`} className="border-b border-[var(--line)] border-opacity-30">
                  <td className="px-2 py-1 font-medium text-[var(--text)]">{iface.iface || "-"}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{iface.type || "-"}</td>
                  <td className="px-2 py-1 font-mono text-[10px] text-[var(--muted)]">
                    {iface.cidr || iface.address || "-"}
                  </td>
                  <td className="px-2 py-1 font-mono text-[10px] text-[var(--muted)]">
                    {iface.gateway || "-"}
                  </td>
                  <td className="px-2 py-1 text-[var(--muted)]">{iface.bridge_ports || "-"}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{iface.method || "-"}</td>
                  <td className="px-2 py-1">
                    <Badge status={iface.active === 1 ? "ok" : "bad"} size="sm" />
                  </td>
                  <td className="px-2 py-1">
                    <span className={iface.autostart === 1 ? "text-[var(--ok)]" : "text-[var(--muted)]"}>
                      {iface.autostart === 1 ? "Yes" : "No"}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="text-xs text-[var(--muted)]">No network interfaces returned.</p>
      )}
    </Card>
  );
}
