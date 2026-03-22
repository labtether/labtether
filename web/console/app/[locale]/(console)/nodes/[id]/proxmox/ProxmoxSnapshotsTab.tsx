"use client";

import { Card } from "../../../../../components/ui/Card";
import type { ProxmoxDetails } from "../nodeDetailTypes";
import { ProxmoxSnapshotsSection } from "../ProxmoxSnapshotsSection";

type Props = {
  proxmoxDetails: ProxmoxDetails;
  effectiveKind: string;
  canRunProxmoxQuickActions: boolean;
  proxmoxActionRunning: boolean;
  onRunProxmoxQuickAction: (actionID: string, params?: Record<string, string>) => void;
};

export function ProxmoxSnapshotsTab({
  proxmoxDetails,
  effectiveKind,
  canRunProxmoxQuickActions,
  proxmoxActionRunning,
  onRunProxmoxQuickAction,
}: Props) {
  return (
    <Card>
      <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Snapshots</h2>
      <ProxmoxSnapshotsSection
        proxmoxDetails={proxmoxDetails}
        effectiveKind={effectiveKind}
        canRunProxmoxQuickActions={canRunProxmoxQuickActions}
        proxmoxActionRunning={proxmoxActionRunning}
        onRunProxmoxQuickAction={onRunProxmoxQuickAction}
      />
    </Card>
  );
}
