import dagre from "@dagrejs/dagre";
import type { Asset } from "../../../../console/models";
import type { ClusterPlacement, GuestPlacement } from "./clusterTopologyFlowPlacements";

function nodeYTop(
  graph: InstanceType<typeof dagre.graphlib.Graph>,
  nodeID: string,
  nodeHeight: number,
  fallbackTop: number,
): number {
  const node = graph.node(nodeID) as { y?: number } | undefined;
  if (typeof node?.y === "number" && Number.isFinite(node.y)) {
    return node.y - (nodeHeight / 2);
  }
  return fallbackTop;
}

export function projectClusterTopologyFlowLayout(params: {
  clusterPlacements: ClusterPlacement[];
  guestPlacements: GuestPlacement[];
  sortedApiHosts: Asset[];
  missingTargets: string[];
  apiNodeIDByAssetID: Map<string, string>;
  missingNodeIDByTarget: Map<string, string>;
  laneTopY: number;
  clusterNodeWidth: number;
  clusterNodeHeight: number;
  guestNodeWidth: number;
  guestNodeHeight: number;
  apiNodeWidth: number;
  apiNodeHeight: number;
}): {
  clusterYByID: Map<string, number>;
  guestYByID: Map<string, number>;
  apiYByID: Map<string, number>;
  missingYByID: Map<string, number>;
  shiftedY: (value: number) => number;
} {
  const {
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
  } = params;

  // Dagre computes vertical ordering that minimizes crossings; we preserve fixed
  // horizontal lanes by projecting output Y positions onto lane X coordinates.
  const graph = new dagre.graphlib.Graph();
  graph.setGraph({
    rankdir: "LR",
    nodesep: 42,
    ranksep: 140,
    edgesep: 20,
    marginx: 20,
    marginy: 20,
    ranker: "network-simplex",
  });
  graph.setDefaultEdgeLabel(() => ({}));

  for (const cluster of clusterPlacements) {
    graph.setNode(cluster.clusterNodeID, { width: clusterNodeWidth, height: clusterNodeHeight });
  }
  for (const guest of guestPlacements) {
    graph.setNode(guest.guestNodeID, { width: guestNodeWidth, height: guestNodeHeight });
  }
  for (const host of sortedApiHosts) {
    graph.setNode(`api-host:${host.id}`, { width: apiNodeWidth, height: apiNodeHeight });
  }
  for (const targetID of missingTargets) {
    graph.setNode(`api-missing:${targetID}`, { width: apiNodeWidth, height: apiNodeHeight });
  }

  for (const guest of guestPlacements) {
    graph.setEdge(guest.clusterNodeID, guest.guestNodeID, { minlen: 1, weight: 3 });
    if (guest.linkedHostID) {
      const hostNodeID = apiNodeIDByAssetID.get(guest.linkedHostID);
      if (hostNodeID) {
        graph.setEdge(guest.guestNodeID, hostNodeID, { minlen: 2, weight: 5 });
      }
      continue;
    }
    if (guest.mapping) {
      const missingNodeID = missingNodeIDByTarget.get(guest.mapping.target_asset_id);
      if (missingNodeID) {
        graph.setEdge(guest.guestNodeID, missingNodeID, { minlen: 2, weight: 5 });
      }
    }
  }

  // Keep stable ordering for disconnected hosts/missing targets.
  for (let index = 1; index < sortedApiHosts.length; index++) {
    graph.setEdge(
      `api-host:${sortedApiHosts[index - 1]!.id}`,
      `api-host:${sortedApiHosts[index]!.id}`,
      { minlen: 1, weight: 0.08 },
    );
  }
  for (let index = 1; index < missingTargets.length; index++) {
    graph.setEdge(
      `api-missing:${missingTargets[index - 1]!}`,
      `api-missing:${missingTargets[index]!}`,
      { minlen: 1, weight: 0.08 },
    );
  }

  dagre.layout(graph);

  const clusterYByID = new Map<string, number>();
  const guestYByID = new Map<string, number>();
  const apiYByID = new Map<string, number>();
  const missingYByID = new Map<string, number>();

  for (let index = 0; index < clusterPlacements.length; index++) {
    const cluster = clusterPlacements[index]!;
    clusterYByID.set(
      cluster.clusterNodeID,
      nodeYTop(graph, cluster.clusterNodeID, clusterNodeHeight, laneTopY + (index * clusterNodeHeight)),
    );
  }
  for (let index = 0; index < guestPlacements.length; index++) {
    const guest = guestPlacements[index]!;
    guestYByID.set(
      guest.guestNodeID,
      nodeYTop(graph, guest.guestNodeID, guestNodeHeight, laneTopY + (index * (guestNodeHeight * 0.8))),
    );
  }
  for (let index = 0; index < sortedApiHosts.length; index++) {
    const host = sortedApiHosts[index]!;
    const nodeID = `api-host:${host.id}`;
    apiYByID.set(
      nodeID,
      nodeYTop(graph, nodeID, apiNodeHeight, laneTopY + (index * apiNodeHeight)),
    );
  }
  for (let index = 0; index < missingTargets.length; index++) {
    const targetID = missingTargets[index]!;
    const nodeID = `api-missing:${targetID}`;
    missingYByID.set(
      nodeID,
      nodeYTop(graph, nodeID, apiNodeHeight, laneTopY + ((sortedApiHosts.length + index) * apiNodeHeight)),
    );
  }

  const rawYValues = [
    ...clusterYByID.values(),
    ...guestYByID.values(),
    ...apiYByID.values(),
    ...missingYByID.values(),
  ];
  const minY = rawYValues.length > 0 ? Math.min(...rawYValues) : laneTopY;
  const yOffset = laneTopY - minY;
  const shiftedY = (value: number): number => Math.round((value + yOffset) * 100) / 100;

  return {
    clusterYByID,
    guestYByID,
    apiYByID,
    missingYByID,
    shiftedY,
  };
}
