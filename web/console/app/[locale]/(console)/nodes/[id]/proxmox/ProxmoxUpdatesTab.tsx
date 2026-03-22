"use client";

import { Card } from "../../../../../components/ui/Card";
import { useProxmoxList } from "./useProxmoxData";

type UpdatePackage = {
  Package?: string;
  Title?: string;
  OldVersion?: string;
  Version?: string;
  Origin?: string;
  Priority?: string;
  Section?: string;
  Description?: string;
};

type Props = {
  proxmoxNode: string;
  proxmoxCollectorID: string;
};

export function ProxmoxUpdatesTab({ proxmoxNode, proxmoxCollectorID }: Props) {
  const collectorSuffix = proxmoxCollectorID
    ? `?collector_id=${encodeURIComponent(proxmoxCollectorID)}`
    : "";

  const path = proxmoxNode
    ? `/api/proxmox/nodes/${encodeURIComponent(proxmoxNode)}/updates${collectorSuffix}`
    : null;

  const { data: packages, loading, error, refresh } = useProxmoxList<UpdatePackage>(path);

  return (
    <Card>
      <div className="mb-3 flex items-center gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">
          Available Updates{packages.length > 0 ? ` (${packages.length})` : ""}
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
      ) : loading && packages.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">Checking for updates...</p>
      ) : packages.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Package</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Title</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Current</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Available</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Section</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Origin</th>
              </tr>
            </thead>
            <tbody>
              {packages.map((pkg, idx) => (
                <tr key={pkg.Package ?? idx} className="border-b border-[var(--line)] border-opacity-30">
                  <td className="px-2 py-1 font-medium text-[var(--text)]">{pkg.Package || "-"}</td>
                  <td className="max-w-[200px] truncate px-2 py-1 text-[var(--muted)]">
                    {pkg.Title || "-"}
                  </td>
                  <td className="px-2 py-1 font-mono text-[10px] text-[var(--muted)]">
                    {pkg.OldVersion || "-"}
                  </td>
                  <td className="px-2 py-1 font-mono text-[10px] text-[var(--ok)]">
                    {pkg.Version || "-"}
                  </td>
                  <td className="px-2 py-1 text-[var(--muted)]">{pkg.Section || "-"}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{pkg.Origin || "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="flex items-center gap-2 py-4">
          <span className="text-xs text-[var(--ok)]">System is up to date.</span>
        </div>
      )}
    </Card>
  );
}
