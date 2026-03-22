"use client";

import { Card } from "../../../../../components/ui/Card";
import type {
  ClusterStatusEntry,
  NetworkInterface,
  ProxmoxDetails,
} from "../nodeDetailTypes";
import { ProxmoxStorageNetworkSections } from "../ProxmoxStorageNetworkSections";

type Props = {
  proxmoxDetails: ProxmoxDetails;
  clusterStatus: ClusterStatusEntry[];
  networkInterfaces: NetworkInterface[];
};

export function ProxmoxStorageTab({
  proxmoxDetails,
  clusterStatus,
  networkInterfaces,
}: Props) {
  return (
    <Card>
      <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Storage</h2>
      <ProxmoxStorageNetworkSections
        proxmoxDetails={proxmoxDetails}
        clusterStatus={clusterStatus}
        networkInterfaces={networkInterfaces}
      />
    </Card>
  );
}
