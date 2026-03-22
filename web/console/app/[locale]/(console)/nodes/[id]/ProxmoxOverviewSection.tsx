"use client";

import { useMemo, useState } from "react";
import type { ProxmoxDetails } from "./nodeDetailTypes";
import { formatProxmoxValue } from "./proxmoxFormatters";

type ProxmoxOverviewSectionProps = {
  proxmoxDetails: ProxmoxDetails;
  proxmoxNode: string;
  proxmoxVMID: string;
  effectiveKind: string;
};

export function ProxmoxOverviewSection({
  proxmoxDetails,
  proxmoxNode,
  proxmoxVMID,
  effectiveKind,
}: ProxmoxOverviewSectionProps) {
  const [showAllConfig, setShowAllConfig] = useState(false);

  const proxmoxConfigEntries = useMemo(() => {
    const config = proxmoxDetails.config ?? {};
    return Object.entries(config)
      .filter(([, value]) => value !== null && value !== undefined && String(value).trim() !== "")
      .sort(([left], [right]) => left.localeCompare(right));
  }, [proxmoxDetails]);

  return (
    <>
      <div className="divide-y divide-[var(--line)]">
        <div className="flex items-start justify-between gap-4 py-3">
          <div className="flex flex-col gap-0.5">
            <span className="text-sm font-medium text-[var(--text)]">Resource</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="rounded-lg border border-[var(--line)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">{proxmoxDetails.kind || effectiveKind || "unknown"}</span>
          </div>
        </div>
        <div className="flex items-start justify-between gap-4 py-3">
          <div className="flex flex-col gap-0.5">
            <span className="text-sm font-medium text-[var(--text)]">Node</span>
          </div>
          <div className="flex items-center gap-2">
            <code>{proxmoxDetails.node || proxmoxNode || "unknown"}</code>
          </div>
        </div>
        {proxmoxDetails.vmid || proxmoxVMID ? (
          <div className="flex items-start justify-between gap-4 py-3">
            <div className="flex flex-col gap-0.5">
              <span className="text-sm font-medium text-[var(--text)]">VMID</span>
            </div>
            <div className="flex items-center gap-2">
              <code>{proxmoxDetails.vmid || proxmoxVMID}</code>
            </div>
          </div>
        ) : null}
        {proxmoxDetails.version ? (
          <div className="flex items-start justify-between gap-4 py-3">
            <div className="flex flex-col gap-0.5">
              <span className="text-sm font-medium text-[var(--text)]">Cluster Version</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-[var(--muted)]">{proxmoxDetails.version}</span>
            </div>
          </div>
        ) : null}
        {proxmoxDetails.fetched_at ? (
          <div className="flex items-start justify-between gap-4 py-3">
            <div className="flex flex-col gap-0.5">
              <span className="text-sm font-medium text-[var(--text)]">Fetched</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-[var(--muted)]">{new Date(proxmoxDetails.fetched_at).toLocaleString()}</span>
            </div>
          </div>
        ) : null}
      </div>

      {proxmoxDetails.warnings?.length ? (
        <div className="flex flex-col items-start gap-2 py-3">
          <p className="text-sm font-medium text-[var(--text)]">Partial data returned</p>
          <ul className="w-full divide-y divide-[var(--line)]">
            {proxmoxDetails.warnings.map((warning, idx) => (
              <li key={`${warning}-${idx}`} className="flex items-center justify-between gap-3 py-2.5">
                <div>
                  <span className="text-xs text-[var(--bad)]">{warning}</span>
                </div>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="space-y-2">
        <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">Configuration</p>
        {proxmoxConfigEntries.length > 0 ? (
          <>
            <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5">
              {(showAllConfig ? proxmoxConfigEntries : proxmoxConfigEntries.slice(0, 36)).map(([key, value]) => (
                <div key={key}>
                  <dt className="text-xs text-[var(--muted)]">{key}</dt>
                  <dd className="text-xs text-[var(--text)]">{formatProxmoxValue(value)}</dd>
                </div>
              ))}
            </dl>
            {proxmoxConfigEntries.length > 36 ? (
              <button
                className="mt-2 text-xs text-[var(--accent)] hover:underline"
                onClick={() => {
                  setShowAllConfig((current) => !current);
                }}
              >
                {showAllConfig ? "Show less" : `Show all ${proxmoxConfigEntries.length} keys`}
              </button>
            ) : null}
          </>
        ) : (
          <p className="text-xs text-[var(--muted)]">No config values returned.</p>
        )}
      </div>
    </>
  );
}
