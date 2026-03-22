"use client";

import { Card } from "../../../../../components/ui/Card";

type Props = {
  proxmoxNode: string;
  proxmoxVMID: string;
  effectiveKind: string;
  proxmoxCollectorID: string;
};

export function ProxmoxConsoleTab({
  proxmoxNode,
  proxmoxVMID,
  effectiveKind,
  proxmoxCollectorID,
}: Props) {
  const isVM = effectiveKind === "qemu";
  const isCT = effectiveKind === "lxc";

  const collectorSuffix = proxmoxCollectorID
    ? `&collector_id=${encodeURIComponent(proxmoxCollectorID)}`
    : "";

  let consoleURL: string | null = null;
  if (proxmoxNode && proxmoxVMID) {
    if (isVM) {
      consoleURL = `/api/proxmox/nodes/${encodeURIComponent(proxmoxNode)}/qemu/${encodeURIComponent(proxmoxVMID)}/vncproxy?console=1${collectorSuffix}`;
    } else if (isCT) {
      consoleURL = `/api/proxmox/nodes/${encodeURIComponent(proxmoxNode)}/lxc/${encodeURIComponent(proxmoxVMID)}/vncproxy?console=1${collectorSuffix}`;
    }
  }

  const consoleType = isVM ? "QEMU VNC" : isCT ? "LXC Console" : "Console";

  return (
    <Card>
      <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Console</h2>
      <div className="space-y-3">
        <div className="flex items-center justify-between gap-3 py-2">
          <div>
            <p className="text-sm font-medium text-[var(--text)]">Console Type</p>
            <p className="text-xs text-[var(--muted)]">{consoleType}</p>
          </div>
          <span className="rounded-lg border border-[var(--line)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">
            {effectiveKind}
          </span>
        </div>
        {proxmoxNode && proxmoxVMID ? (
          <div className="flex items-center justify-between gap-3 py-2">
            <div>
              <p className="text-xs text-[var(--muted)]">Node: <code>{proxmoxNode}</code></p>
              <p className="text-xs text-[var(--muted)]">VMID: <code>{proxmoxVMID}</code></p>
            </div>
            {consoleURL ? (
              <a
                href={consoleURL}
                target="_blank"
                rel="noopener noreferrer"
                className="rounded border border-[var(--line)] bg-[var(--surface)] px-3 py-1.5 text-xs font-medium text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
              >
                Open Console
              </a>
            ) : null}
          </div>
        ) : (
          <p className="text-xs text-[var(--muted)]">
            Node and VMID are required to open a console.
          </p>
        )}
        <p className="text-[10px] text-[var(--muted)]">
          Console access proxies through the Proxmox VNC endpoint via the LabTether backend.
        </p>
      </div>
    </Card>
  );
}
