"use client";

import { formatBytes } from "../../../../console/formatters";
import { Badge } from "../../../../components/ui/Badge";
import type {
  ClusterStatusEntry,
  NetworkInterface,
  ProxmoxDetails,
} from "./nodeDetailTypes";
import { formatProxmoxEpoch } from "./proxmoxFormatters";

type ProxmoxStorageNetworkSectionsProps = {
  proxmoxDetails: ProxmoxDetails;
  clusterStatus: ClusterStatusEntry[];
  networkInterfaces: NetworkInterface[];
};

export function ProxmoxStorageNetworkSections({
  proxmoxDetails,
  clusterStatus,
  networkInterfaces,
}: ProxmoxStorageNetworkSectionsProps) {
  return (
    <>
      {proxmoxDetails.zfs_pools && proxmoxDetails.zfs_pools.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">
            ZFS Pools ({proxmoxDetails.zfs_pools.length})
          </p>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Name</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Size</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Free</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Alloc</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Health</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Frag%</th>
                </tr>
              </thead>
              <tbody>
                {proxmoxDetails.zfs_pools.map((pool, idx) => (
                  <tr key={pool.name || idx} className="border-b border-[var(--line)] border-opacity-30">
                    <td className="px-2 py-1 text-[var(--text)]">{pool.name || "unknown"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{pool.size != null ? formatBytes(pool.size) : "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{pool.free != null ? formatBytes(pool.free) : "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{pool.alloc != null ? formatBytes(pool.alloc) : "-"}</td>
                    <td className="px-2 py-1">
                      <span className={`px-1.5 py-0.5 rounded text-[10px] ${
                        pool.health === "ONLINE"
                          ? "bg-[var(--ok-glow)] text-[var(--ok)]"
                          : pool.health === "DEGRADED"
                            ? "bg-[var(--warn-glow)] text-[var(--warn)]"
                            : pool.health === "FAULTED"
                              ? "bg-[var(--bad-glow)] text-[var(--bad)]"
                              : "bg-[var(--hover)] text-[var(--text)]"
                      }`}>{pool.health || "unknown"}</span>
                    </td>
                    <td className="px-2 py-1 text-[var(--muted)]">{pool.frag != null ? `${pool.frag}%` : "-"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}

      {proxmoxDetails.storage_content && proxmoxDetails.storage_content.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">
            Storage Contents ({proxmoxDetails.storage_content.length})
          </p>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Vol ID</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Content Type</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Format</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Size</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Created</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">VMID</th>
                </tr>
              </thead>
              <tbody>
                {proxmoxDetails.storage_content.map((item, idx) => (
                  <tr key={item.volid || idx} className="border-b border-[var(--line)] border-opacity-30">
                    <td className="max-w-[300px] break-all px-2 py-1 font-mono text-[10px] text-[var(--text)]">{item.volid || "-"}</td>
                    <td className="px-2 py-1">
                      <span className={`px-1.5 py-0.5 rounded text-[10px] ${
                        item.content === "iso"
                          ? "bg-[var(--accent-subtle)] text-[var(--accent-text)]"
                          : item.content === "vztmpl"
                            ? "bg-[rgba(var(--accent-rgb),0.1)] text-[var(--accent)]"
                            : item.content === "backup"
                              ? "bg-[var(--warn-glow)] text-[var(--warn)]"
                              : item.content === "images"
                                ? "bg-[var(--ok-glow)] text-[var(--ok)]"
                                : "bg-[var(--hover)] text-[var(--text)]"
                      }`}>{item.content || "-"}</span>
                    </td>
                    <td className="px-2 py-1 text-[var(--muted)]">{item.format || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{item.size != null ? formatBytes(item.size) : "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{formatProxmoxEpoch(item.ctime)}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{item.vmid ? String(item.vmid) : "-"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}

      {clusterStatus.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">Cluster</p>
          <ul className="divide-y divide-[var(--line)]">
            {clusterStatus.map((entry, idx) => (
              <li key={`${entry.name ?? "entry"}-${idx}`} className="flex items-center justify-between gap-3 py-2.5">
                <div>
                  <span className="text-sm font-medium text-[var(--text)]">{entry.name || "unknown"}</span>
                  <code className="block text-xs text-[var(--muted)]">{entry.type}{entry.ip ? ` / ${entry.ip}` : ""}</code>
                </div>
                <div className="flex items-center gap-2">
                  {entry.type === "node" ? (
                    <Badge status={entry.online === 1 ? "ok" : "bad"} />
                  ) : null}
                  {entry.quorate != null ? (
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--line)] ${entry.quorate === 1 ? "text-[var(--ok)]" : "text-[var(--bad)]"}`}>
                      {entry.quorate === 1 ? "quorate" : "no quorum"}
                    </span>
                  ) : null}
                  {entry.local === 1 ? (
                    <span className="text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]">local</span>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {networkInterfaces.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">Network Interfaces</p>
          <ul className="divide-y divide-[var(--line)]">
            {networkInterfaces.slice(0, 20).map((iface, idx) => (
              <li key={`${iface.iface ?? "iface"}-${idx}`} className="flex items-center justify-between gap-3 py-2.5">
                <div>
                  <span className="text-sm font-medium text-[var(--text)]">{iface.iface || "unknown"}</span>
                  <code className="block text-xs text-[var(--muted)]">
                    {iface.type}{iface.cidr ? ` ${iface.cidr}` : iface.address ? ` ${iface.address}` : ""}
                    {iface.bridge_ports ? ` (ports: ${iface.bridge_ports})` : ""}
                  </code>
                </div>
                <div className="flex items-center gap-2">
                  <Badge status={iface.active === 1 ? "ok" : "bad"} />
                  {iface.gateway ? (
                    <span className="rounded-lg border border-[var(--line)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">gw: {iface.gateway}</span>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </>
  );
}
