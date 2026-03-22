"use client";

import { Button } from "../../../../components/ui/Button";
import type { ProxmoxDetails } from "./nodeDetailTypes";
import { formatProxmoxEpoch } from "./proxmoxFormatters";

type ProxmoxSnapshotsSectionProps = {
  proxmoxDetails: ProxmoxDetails;
  effectiveKind: string;
  canRunProxmoxQuickActions: boolean;
  proxmoxActionRunning: boolean;
  onRunProxmoxQuickAction: (actionID: string, params?: Record<string, string>) => void;
};

export function ProxmoxSnapshotsSection({
  proxmoxDetails,
  effectiveKind,
  canRunProxmoxQuickActions,
  proxmoxActionRunning,
  onRunProxmoxQuickAction,
}: ProxmoxSnapshotsSectionProps) {
  return (
    <div className="space-y-2">
      <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">Snapshots</p>
      {proxmoxDetails.snapshots && proxmoxDetails.snapshots.length > 0 ? (
        <ul className="divide-y divide-[var(--line)]">
          {proxmoxDetails.snapshots.slice(0, 20).map((snapshot, idx) => (
            <li key={`${snapshot.name ?? "snapshot"}-${snapshot.snaptime ?? idx}`} className="flex items-center justify-between gap-3 py-2.5">
              <div>
                <span className="text-sm font-medium text-[var(--text)]">{snapshot.name || "snapshot"}</span>
                <code className="block text-xs text-[var(--muted)]">{snapshot.parent || "root"}</code>
              </div>
              <div className="flex items-center gap-2">
                {snapshot.vmstate ? <span className="rounded-lg border border-[var(--line)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">{snapshot.vmstate}</span> : null}
                <span className="text-xs text-[var(--muted)]">{formatProxmoxEpoch(snapshot.snaptime)}</span>
                {canRunProxmoxQuickActions && snapshot.name ? (
                  <>
                    <Button
                      size="sm"
                      disabled={proxmoxActionRunning}
                      onClick={() => {
                        if (confirm(`Rollback to snapshot "${snapshot.name}"? This may stop the VM/CT.`)) {
                          const actionID = effectiveKind === "lxc" ? "ct.snapshot.rollback" : "vm.snapshot.rollback";
                          onRunProxmoxQuickAction(actionID, { snapshot_name: snapshot.name! });
                        }
                      }}
                    >
                      Rollback
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      disabled={proxmoxActionRunning}
                      onClick={() => {
                        if (confirm(`Delete snapshot "${snapshot.name}"? This cannot be undone.`)) {
                          const actionID = effectiveKind === "lxc" ? "ct.snapshot.delete" : "vm.snapshot.delete";
                          onRunProxmoxQuickAction(actionID, { snapshot_name: snapshot.name! });
                        }
                      }}
                    >
                      Delete
                    </Button>
                  </>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-xs text-[var(--muted)]">No snapshots returned.</p>
      )}
    </div>
  );
}
