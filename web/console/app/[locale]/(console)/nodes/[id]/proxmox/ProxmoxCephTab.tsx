"use client";

import { Card } from "../../../../../components/ui/Card";
import type { ProxmoxDetails } from "../nodeDetailTypes";
import { ProxmoxCephSections } from "../ProxmoxCephSections";

type Props = {
  proxmoxDetails: ProxmoxDetails;
};

export function ProxmoxCephTab({ proxmoxDetails }: Props) {
  return (
    <Card>
      <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Ceph</h2>
      <div className="space-y-6">
        <ProxmoxCephSections proxmoxDetails={proxmoxDetails} />
      </div>
    </Card>
  );
}
