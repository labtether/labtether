import type { Asset } from "../../../../console/models";
import { normalizeAssetName } from "./clusterTopologyUtils";

export type ClusterStatusEntry = {
  name?: string;
  ip?: string;
  online?: number;
  local?: number;
};

export type GuestRunsOnDependency = {
  target_asset_id: string;
  metadata?: Record<string, string>;
};

export type ClusterPlacement = {
  clusterNodeID: string;
  nodeName: string;
  nodeIP: string;
  nodeOnline: boolean;
  nodeLocal: boolean;
  guestCount: number;
  nodeAssetID?: string;
};

export type GuestPlacement = {
  guest: Asset;
  guestNodeID: string;
  clusterNodeID: string;
  nodeOnline: boolean;
  guestSelected: boolean;
  guestStatus: string;
  mapping: GuestRunsOnDependency | null;
  linkedHostID?: string;
  linkedHostName?: string;
  isAuto: boolean;
};

export function deriveClusterTopologyPlacements(params: {
  apiHosts: Asset[];
  apiHostsByID: Map<string, Asset>;
  guestAssets: Asset[];
  guestRunsOn: Record<string, GuestRunsOnDependency | null>;
  guestsByNode: Map<string, Asset[]>;
  selectedGuestID: string;
  nodeEntries: ClusterStatusEntry[];
  proxmoxNodeAssetIDByName: Map<string, string>;
}): {
  sortedApiHosts: Asset[];
  clusterPlacements: ClusterPlacement[];
  guestPlacements: GuestPlacement[];
  missingTargets: string[];
  apiLinkCountByHostID: Map<string, number>;
  apiNodeIDByAssetID: Map<string, string>;
  missingNodeIDByTarget: Map<string, string>;
} {
  const {
    apiHosts,
    apiHostsByID,
    guestAssets,
    guestRunsOn,
    guestsByNode,
    selectedGuestID,
    nodeEntries,
    proxmoxNodeAssetIDByName,
  } = params;

  const sortedNodes = [...nodeEntries].sort((a, b) => (a.name ?? "").localeCompare(b.name ?? ""));
  const sortedApiHosts = [...apiHosts].sort((a, b) => a.name.localeCompare(b.name));

  const clusterPlacements: ClusterPlacement[] = [];
  const guestPlacements: GuestPlacement[] = [];
  const missingTargetIDs = new Set<string>();

  const apiLinkCountByHostID = new Map<string, number>();
  for (const guest of guestAssets) {
    const targetID = guestRunsOn[guest.id]?.target_asset_id;
    if (!targetID) {
      continue;
    }
    apiLinkCountByHostID.set(targetID, (apiLinkCountByHostID.get(targetID) ?? 0) + 1);
  }

  for (const node of sortedNodes) {
    const nodeName = (node.name ?? "unknown").trim() || "unknown";
    const nodeKey = normalizeAssetName(nodeName);
    const clusterNodeID = `cluster-node:${nodeName}`;

    const nodeGuests = [...(guestsByNode.get(nodeName) ?? [])].sort((left, right) => {
      const leftMapping = guestRunsOn[left.id] ?? null;
      const rightMapping = guestRunsOn[right.id] ?? null;
      const leftHost = leftMapping ? apiHostsByID.get(leftMapping.target_asset_id)?.name ?? leftMapping.target_asset_id : "~";
      const rightHost = rightMapping ? apiHostsByID.get(rightMapping.target_asset_id)?.name ?? rightMapping.target_asset_id : "~";
      const hostDiff = leftHost.localeCompare(rightHost);
      if (hostDiff !== 0) {
        return hostDiff;
      }
      return left.name.localeCompare(right.name);
    });

    clusterPlacements.push({
      clusterNodeID,
      nodeName,
      nodeIP: node.ip || "no IP detected",
      nodeOnline: node.online === 1,
      nodeLocal: node.local === 1,
      guestCount: nodeGuests.length,
      nodeAssetID: proxmoxNodeAssetIDByName.get(nodeKey),
    });

    for (const guest of nodeGuests) {
      const guestNodeID = `guest:${guest.id}`;
      const guestStatus = (guest.metadata?.status ?? guest.status ?? "").toLowerCase();
      const mapping = guestRunsOn[guest.id] ?? null;
      const linkedHost = mapping ? apiHostsByID.get(mapping.target_asset_id) : undefined;
      const isAuto = mapping?.metadata?.binding === "auto";
      const guestSelected = guest.id === selectedGuestID;

      if (mapping && !linkedHost) {
        missingTargetIDs.add(mapping.target_asset_id);
      }

      guestPlacements.push({
        guest,
        guestNodeID,
        clusterNodeID,
        nodeOnline: node.online === 1,
        guestSelected,
        guestStatus,
        mapping,
        linkedHostID: linkedHost?.id,
        linkedHostName: linkedHost?.name,
        isAuto,
      });
    }
  }

  const missingTargets = [...missingTargetIDs].sort((a, b) => a.localeCompare(b));

  const apiNodeIDByAssetID = new Map<string, string>();
  for (const host of sortedApiHosts) {
    apiNodeIDByAssetID.set(host.id, `api-host:${host.id}`);
  }

  const missingNodeIDByTarget = new Map<string, string>();
  for (const targetID of missingTargets) {
    missingNodeIDByTarget.set(targetID, `api-missing:${targetID}`);
  }

  return {
    sortedApiHosts,
    clusterPlacements,
    guestPlacements,
    missingTargets,
    apiLinkCountByHostID,
    apiNodeIDByAssetID,
    missingNodeIDByTarget,
  };
}
