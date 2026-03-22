"use client";

import { Card } from "../../../../../components/ui/Card";
import { useProxmoxList } from "./useProxmoxData";

type ReplicationJob = {
  id?: string;
  type?: string;
  target?: string;
  schedule?: string;
  enabled?: boolean | number;
  state?: string;
  last_sync?: number;
  next_sync?: number;
  duration?: number;
  fail_count?: number;
  comment?: string;
  vmid?: number;
};

type Props = {
  proxmoxNode: string;
  proxmoxCollectorID: string;
};

export function ProxmoxReplicationTab({ proxmoxNode, proxmoxCollectorID }: Props) {
  const collectorSuffix = proxmoxCollectorID
    ? `?collector_id=${encodeURIComponent(proxmoxCollectorID)}`
    : "";

  const path = proxmoxNode
    ? `/api/proxmox/nodes/${encodeURIComponent(proxmoxNode)}/replication${collectorSuffix}`
    : null;

  const { data: jobs, loading, error, refresh } = useProxmoxList<ReplicationJob>(path);

  return (
    <Card>
      <div className="mb-3 flex items-center gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">
          Replication{jobs.length > 0 ? ` (${jobs.length})` : ""}
        </h2>
        <button
          className="ml-auto text-xs text-[var(--accent)] hover:underline"
          onClick={refresh}
          disabled={loading}
        >
          {loading ? "Loading..." : "Refresh"}
        </button>
      </div>
      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : loading && jobs.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">Loading replication jobs...</p>
      ) : jobs.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">ID</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">VMID</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Type</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Target</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Schedule</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">State</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Last Sync</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Fails</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Enabled</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((job, idx) => {
                const enabled = job.enabled === true || job.enabled === 1;
                return (
                  <tr key={job.id ?? idx} className="border-b border-[var(--line)] border-opacity-30">
                    <td className="px-2 py-1 font-mono text-[10px] text-[var(--text)]">{job.id || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">
                      {job.vmid != null ? String(job.vmid) : "-"}
                    </td>
                    <td className="px-2 py-1 text-[var(--muted)]">{job.type || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{job.target || "-"}</td>
                    <td className="px-2 py-1 text-[var(--muted)]">{job.schedule || "-"}</td>
                    <td className="px-2 py-1">
                      {job.state ? (
                        <span
                          className={`rounded px-1.5 py-0.5 text-[10px] ${
                            job.state === "ok"
                              ? "bg-[var(--ok-glow)] text-[var(--ok)]"
                              : job.state === "error"
                                ? "bg-[var(--bad-glow)] text-[var(--bad)]"
                                : "bg-[var(--hover)] text-[var(--text)]"
                          }`}
                        >
                          {job.state}
                        </span>
                      ) : (
                        <span className="text-[var(--muted)]">-</span>
                      )}
                    </td>
                    <td className="px-2 py-1 text-[var(--muted)]">
                      {job.last_sync
                        ? new Date(job.last_sync * 1000).toLocaleString()
                        : "-"}
                    </td>
                    <td className="px-2 py-1 text-[var(--muted)]">
                      {job.fail_count != null ? String(job.fail_count) : "-"}
                    </td>
                    <td className="px-2 py-1">
                      <span className={enabled ? "text-[var(--ok)]" : "text-[var(--bad)]"}>
                        {enabled ? "Yes" : "No"}
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="text-xs text-[var(--muted)]">No replication jobs found.</p>
      )}
    </Card>
  );
}
