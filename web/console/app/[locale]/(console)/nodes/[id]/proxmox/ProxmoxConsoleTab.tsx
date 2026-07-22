"use client";

import { Card } from "../../../../../components/ui/Card";
import { Link } from "../../../../../../i18n/navigation";

type Props = {
  assetId: string;
  proxmoxNode: string;
  proxmoxVMID: string;
  effectiveKind: string;
};

export function ProxmoxConsoleTab({
  assetId,
  proxmoxNode,
  proxmoxVMID,
  effectiveKind,
}: Props) {
  const isVM = effectiveKind === "qemu";
  const isCT = effectiveKind === "lxc";

  const consoleURL = assetId ? `/nodes/${encodeURIComponent(assetId)}?panel=desktop` : null;

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
              <Link
                href={consoleURL}
                className="rounded border border-[var(--line)] bg-[var(--surface)] px-3 py-1.5 text-xs font-medium text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
              >
                Open Secure Console
              </Link>
            ) : null}
          </div>
        ) : (
          <p className="text-xs text-[var(--muted)]">
            Node and VMID are required to open a console.
          </p>
        )}
        <p className="text-[10px] text-[var(--muted)]">
          Console access uses LabTether&apos;s authenticated desktop-session bridge so generated Proxmox tickets stay server-side.
        </p>
      </div>
    </Card>
  );
}
