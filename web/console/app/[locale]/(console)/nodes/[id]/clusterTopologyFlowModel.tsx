import { type CSSProperties } from "react";
import {
  type Edge,
  type Node,
} from "@xyflow/react";
import type { Asset } from "../../../../console/models";
import {
  materializeClusterTopologyFlowElements,
  type ClusterTopologyNodeData,
} from "./clusterTopologyFlowElements";
import { projectClusterTopologyFlowLayout } from "./clusterTopologyFlowLayout";
import {
  deriveClusterTopologyPlacements,
  type ClusterStatusEntry,
  type GuestRunsOnDependency,
} from "./clusterTopologyFlowPlacements";

export type { ClusterTopologyNodeData };

export function buildClusterFlowModel(params: {
  apiHosts: Asset[];
  apiHostsByID: Map<string, Asset>;
  guestAssets: Asset[];
  guestRunsOn: Record<string, GuestRunsOnDependency | null>;
  guestsByNode: Map<string, Asset[]>;
  graphSelectedGuestID: string | null;
  nodeEntries: ClusterStatusEntry[];
  proxmoxNodeAssetIDByName: Map<string, string>;
}): {
  nodes: Node<ClusterTopologyNodeData>[];
  edges: Edge[];
} {
  const {
    apiHosts,
    apiHostsByID,
    guestAssets,
    guestRunsOn,
    guestsByNode,
    graphSelectedGuestID,
    nodeEntries,
    proxmoxNodeAssetIDByName,
  } = params;

  const laneX = { cluster: 28, guest: 360, api: 700 };
  const laneTopY = 36;
  const selectedGuestID = graphSelectedGuestID ?? "";

  const clusterNodeWidth = 250;
  const clusterNodeHeight = 86;
  const guestNodeWidth = 260;
  const guestNodeHeight = 92;
  const apiNodeWidth = 220;
  const apiNodeHeight = 80;

  const baseNodeStyle: CSSProperties = {
    width: clusterNodeWidth,
    borderRadius: 10,
    border: "1px solid var(--line)",
    background: "color-mix(in oklab, var(--panel) 86%, transparent)",
    color: "var(--text)",
    padding: "10px 12px",
    boxShadow: "0 8px 20px rgba(0, 0, 0, 0.22)",
    cursor: "pointer",
  };

  const {
    sortedApiHosts,
    clusterPlacements,
    guestPlacements,
    missingTargets,
    apiLinkCountByHostID,
    apiNodeIDByAssetID,
    missingNodeIDByTarget,
  } = deriveClusterTopologyPlacements({
    apiHosts,
    apiHostsByID,
    guestAssets,
    guestRunsOn,
    guestsByNode,
    selectedGuestID,
    nodeEntries,
    proxmoxNodeAssetIDByName,
  });

  const {
    clusterYByID,
    guestYByID,
    apiYByID,
    missingYByID,
    shiftedY,
  } = projectClusterTopologyFlowLayout({
    clusterPlacements,
    guestPlacements,
    sortedApiHosts,
    missingTargets,
    apiNodeIDByAssetID,
    missingNodeIDByTarget,
    laneTopY,
    clusterNodeWidth,
    clusterNodeHeight,
    guestNodeWidth,
    guestNodeHeight,
    apiNodeWidth,
    apiNodeHeight,
  });

  return materializeClusterTopologyFlowElements({
    baseNodeStyle,
    laneX,
    laneTopY,
    clusterNodeWidth,
    guestNodeWidth,
    apiNodeWidth,
    clusterPlacements,
    guestPlacements,
    sortedApiHosts,
    missingTargets,
    apiLinkCountByHostID,
    apiNodeIDByAssetID,
    missingNodeIDByTarget,
    clusterYByID,
    guestYByID,
    apiYByID,
    missingYByID,
    shiftedY,
  });
}
