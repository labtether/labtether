"use client";

import { formatBytes } from "../../../../console/formatters";
import type { ProxmoxDetails } from "./nodeDetailTypes";

type ProxmoxCephSectionsProps = {
  proxmoxDetails: ProxmoxDetails;
};

export function ProxmoxCephSections({ proxmoxDetails }: ProxmoxCephSectionsProps) {
  return (
    <>
      {proxmoxDetails.ceph_status?.health?.status ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">Ceph Cluster</p>
          <div className="space-y-3">
            <div className="flex items-center gap-3">
              <span className="text-sm font-medium text-[var(--text)]">Health</span>
              <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                proxmoxDetails.ceph_status.health.status === "HEALTH_OK"
                  ? "bg-[var(--ok-glow)] text-[var(--ok)]"
                  : proxmoxDetails.ceph_status.health.status === "HEALTH_WARN"
                    ? "bg-[var(--warn-glow)] text-[var(--warn)]"
                    : "bg-[var(--bad-glow)] text-[var(--bad)]"
              }`}>
                {proxmoxDetails.ceph_status.health.status}
              </span>
            </div>
            {proxmoxDetails.ceph_status.pgmap ? (
              <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5">
                {proxmoxDetails.ceph_status.pgmap.bytes_used != null && proxmoxDetails.ceph_status.pgmap.bytes_total != null ? (
                  <div>
                    <dt className="text-xs text-[var(--muted)]">Storage Used</dt>
                    <dd className="text-xs text-[var(--text)]">
                      {formatBytes(proxmoxDetails.ceph_status.pgmap.bytes_used)} / {formatBytes(proxmoxDetails.ceph_status.pgmap.bytes_total)}
                      {proxmoxDetails.ceph_status.pgmap.bytes_total > 0 ? (
                        <span className="ml-1 text-[var(--muted)]">
                          ({Math.round((proxmoxDetails.ceph_status.pgmap.bytes_used / proxmoxDetails.ceph_status.pgmap.bytes_total) * 100)}%)
                        </span>
                      ) : null}
                    </dd>
                  </div>
                ) : null}
                {proxmoxDetails.ceph_status.pgmap.bytes_avail != null ? (
                  <div>
                    <dt className="text-xs text-[var(--muted)]">Available</dt>
                    <dd className="text-xs text-[var(--text)]">{formatBytes(proxmoxDetails.ceph_status.pgmap.bytes_avail)}</dd>
                  </div>
                ) : null}
                {proxmoxDetails.ceph_status.pgmap.data_bytes != null ? (
                  <div>
                    <dt className="text-xs text-[var(--muted)]">Data Stored</dt>
                    <dd className="text-xs text-[var(--text)]">{formatBytes(proxmoxDetails.ceph_status.pgmap.data_bytes)}</dd>
                  </div>
                ) : null}
                {proxmoxDetails.ceph_status.monmap?.mons ? (
                  <div>
                    <dt className="text-xs text-[var(--muted)]">Monitors</dt>
                    <dd className="text-xs text-[var(--text)]">
                      {proxmoxDetails.ceph_status.monmap.mons.length}
                      <span className="ml-1 text-[var(--muted)]">
                        ({proxmoxDetails.ceph_status.monmap.mons.map((monitor) => monitor.name).join(", ")})
                      </span>
                    </dd>
                  </div>
                ) : null}
              </dl>
            ) : null}
            {proxmoxDetails.ceph_status.pgmap?.pgs_by_state && proxmoxDetails.ceph_status.pgmap.pgs_by_state.length > 0 ? (
              <div>
                <dt className="mb-1 text-xs text-[var(--muted)]">PG States</dt>
                <div className="flex flex-wrap gap-1.5">
                  {proxmoxDetails.ceph_status.pgmap.pgs_by_state.map((pg, idx) => (
                    <span key={idx} className={`px-1.5 py-0.5 rounded text-[10px] ${
                      pg.state_name.includes("active+clean")
                        ? "bg-[var(--ok-glow)] text-[var(--ok)]"
                        : pg.state_name.includes("degraded") || pg.state_name.includes("recovering")
                          ? "bg-[var(--warn-glow)] text-[var(--warn)]"
                          : "bg-[var(--hover)] text-[var(--text)]"
                    }`}>
                      {pg.state_name}: {pg.count}
                    </span>
                  ))}
                </div>
              </div>
            ) : null}
          </div>
        </div>
      ) : null}

      {proxmoxDetails.ceph_osds && proxmoxDetails.ceph_osds.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">
            Ceph OSDs ({proxmoxDetails.ceph_osds.length})
          </p>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">ID</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Name</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Host</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Status</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Class</th>
                  <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Weight</th>
                </tr>
              </thead>
              <tbody>
                {proxmoxDetails.ceph_osds.map((osd, idx) => (
                  <tr key={osd.id ?? idx} className="border-b border-[var(--line)] border-opacity-30">
                    <td className="px-2 py-1 text-[var(--muted)]">{osd.id ?? idx}</td>
                    <td className="px-2 py-1 text-[var(--text)]">{osd.name || `osd.${osd.id ?? idx}`}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{osd.host || "-"}</td>
                    <td className="px-2 py-1">
                      <span className={`px-1.5 py-0.5 rounded text-[10px] ${
                        osd.status === "up"
                          ? "bg-[var(--ok-glow)] text-[var(--ok)]"
                          : "bg-[var(--bad-glow)] text-[var(--bad)]"
                      }`}>{osd.status || "unknown"}</span>
                    </td>
                    <td className="px-2 py-1 text-[var(--muted)]">{osd.device_class || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{osd.crush_weight != null ? osd.crush_weight.toFixed(4) : "-"}</td>
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
