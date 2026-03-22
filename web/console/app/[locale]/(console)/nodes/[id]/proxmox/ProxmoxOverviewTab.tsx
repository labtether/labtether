"use client";

import { Card } from "../../../../../components/ui/Card";
import type { ProxmoxDetails } from "../nodeDetailTypes";
import { ProxmoxOverviewSection } from "../ProxmoxOverviewSection";

type Props = {
  proxmoxDetails: ProxmoxDetails;
  proxmoxNode: string;
  proxmoxVMID: string;
  effectiveKind: string;
};

export function ProxmoxOverviewTab({
  proxmoxDetails,
  proxmoxNode,
  proxmoxVMID,
  effectiveKind,
}: Props) {
  return (
    <Card>
      <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Overview</h2>
      <ProxmoxOverviewSection
        proxmoxDetails={proxmoxDetails}
        proxmoxNode={proxmoxNode}
        proxmoxVMID={proxmoxVMID}
        effectiveKind={effectiveKind}
      />
    </Card>
  );
}
