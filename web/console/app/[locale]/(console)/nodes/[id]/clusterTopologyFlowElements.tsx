import { type CSSProperties, type ReactNode } from "react";
import {
  MarkerType,
  Position,
  type Edge,
  type Node,
} from "@xyflow/react";
import type { Asset } from "../../../../console/models";
import { friendlyTypeLabel } from "../../../../console/taxonomy";
import {
  flowEdgeBadge,
  guestStatusTone,
  sourceBadgeLabel,
  type ClusterFlowWords,
} from "./clusterTopologyUtils";
import type {
  ClusterPlacement,
  GuestPlacement,
} from "./clusterTopologyFlowPlacements";

export type ClusterTopologyNodeData = {
  label: ReactNode;
  assetID?: string;
};

/** A tile's edge: only a problem draws one; everything else is the plain tile. */
function statusStroke(status?: string): string {
  const normalized = (status ?? "").trim().toLowerCase();
  if (normalized === "error" || normalized === "failed" || normalized === "offline") {
    return "var(--bad)";
  }
  return "transparent";
}

const SELECTED_RING = "0 0 0 1.5px var(--text-3), var(--elev-tile)";
const PLACEMENT_STROKE = "color-mix(in srgb, var(--text-3) 45%, transparent)";

export function materializeClusterTopologyFlowElements(params: {
  baseNodeStyle: CSSProperties;
  laneX: {
    cluster: number;
    guest: number;
    api: number;
  };
  laneTopY: number;
  clusterNodeWidth: number;
  guestNodeWidth: number;
  apiNodeWidth: number;
  clusterPlacements: ClusterPlacement[];
  guestPlacements: GuestPlacement[];
  sortedApiHosts: Asset[];
  missingTargets: string[];
  apiLinkCountByHostID: Map<string, number>;
  apiNodeIDByAssetID: Map<string, string>;
  missingNodeIDByTarget: Map<string, string>;
  clusterYByID: Map<string, number>;
  guestYByID: Map<string, number>;
  apiYByID: Map<string, number>;
  missingYByID: Map<string, number>;
  shiftedY: (value: number) => number;
  words: ClusterFlowWords;
}): {
  nodes: Node<ClusterTopologyNodeData>[];
  edges: Edge[];
} {
  const {
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
    words,
  } = params;

  const flowNodes: Node<ClusterTopologyNodeData>[] = [];
  const flowEdges: Edge[] = [];

  for (const cluster of clusterPlacements) {
    flowNodes.push({
      id: cluster.clusterNodeID,
      position: {
        x: laneX.cluster,
        y: shiftedY(clusterYByID.get(cluster.clusterNodeID) ?? laneTopY),
      },
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      data: {
        label: (
          <div className="di-ct-tile">
            <span className="di-ct-nm">
              {cluster.nodeOnline ? null : <span className="di-dot bad" aria-hidden="true" />}
              {cluster.nodeName}
            </span>
            <span className="di-ct-sub">{[cluster.nodeLocal ? words.local : "", cluster.nodeIP].filter(Boolean).join(" · ")}</span>
            <span className="di-ct-sub">{words.guests(cluster.guestCount)}</span>
          </div>
        ),
        assetID: cluster.nodeAssetID,
      },
      style: {
        ...baseNodeStyle,
        borderColor: statusStroke(cluster.nodeOnline ? "online" : "offline"),
        width: clusterNodeWidth,
      },
    });
  }

  for (const guest of guestPlacements) {
    flowNodes.push({
      id: guest.guestNodeID,
      position: {
        x: laneX.guest,
        y: shiftedY(guestYByID.get(guest.guestNodeID) ?? laneTopY),
      },
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      data: {
        label: (
          <div className="di-ct-tile">
            <span className="di-ct-nm">
              {guestStatusTone(guest.guestStatus) === "bad" ? <span className="di-dot bad" aria-hidden="true" /> : null}
              {guest.guest.name}
            </span>
            <span className="di-ct-sub">
              {[friendlyTypeLabel(guest.guest.type), guest.guestStatus && guestStatusTone(guest.guestStatus) !== "" ? guest.guestStatus : "", guest.isAuto ? words.auto : ""].filter(Boolean).join(" · ")}
            </span>
            {guest.linkedHostName ? <span className="di-ct-sub">{words.linkedTo(guest.linkedHostName)}</span> : null}
          </div>
        ),
        assetID: guest.guest.id,
      },
      style: {
        ...baseNodeStyle,
        borderColor: statusStroke(guest.guestStatus),
        width: guestNodeWidth,
        boxShadow: guest.guestSelected ? SELECTED_RING : baseNodeStyle.boxShadow,
      },
    });
  }

  for (const host of sortedApiHosts) {
    const nodeID = `api-host:${host.id}`;
    const linkedCount = apiLinkCountByHostID.get(host.id) ?? 0;
    flowNodes.push({
      id: nodeID,
      position: {
        x: laneX.api,
        y: shiftedY(apiYByID.get(nodeID) ?? laneTopY),
      },
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      data: {
        label: (
          <div className="di-ct-tile">
            <span className="di-ct-nm">{host.name}</span>
            <span className="di-ct-sub">{[sourceBadgeLabel(host.source), linkedCount > 0 ? words.links(linkedCount) : ""].filter(Boolean).join(" · ")}</span>
          </div>
        ),
        assetID: host.id,
      },
      style: {
        ...baseNodeStyle,
        width: apiNodeWidth,
      },
    });
  }

  for (const targetID of missingTargets) {
    const nodeID = `api-missing:${targetID}`;
    flowNodes.push({
      id: nodeID,
      position: {
        x: laneX.api,
        y: shiftedY(missingYByID.get(nodeID) ?? laneTopY),
      },
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      data: {
        label: (
          <div className="di-ct-tile">
            <span className="di-ct-nm di-bad">{words.missingTarget}</span>
            <span className="di-ct-sub di-ct-wrap">{targetID}</span>
          </div>
        ),
      },
      style: {
        ...baseNodeStyle,
        borderColor: "var(--bad)",
        width: apiNodeWidth,
        cursor: "default",
      },
    });
  }

  for (const guest of guestPlacements) {
    flowEdges.push({
      id: `edge-placement:${guest.clusterNodeID.slice("cluster-node:".length)}:${guest.guest.id}`,
      source: guest.clusterNodeID,
      target: guest.guestNodeID,
      type: "smoothstep",
      style: {
        stroke: guest.guestSelected ? "var(--text-2)" : PLACEMENT_STROKE,
        strokeWidth: guest.guestSelected ? 2 : 1.25,
      },
    });

    if (!guest.mapping) {
      continue;
    }

    if (guest.linkedHostID) {
      const hostNodeID = apiNodeIDByAssetID.get(guest.linkedHostID);
      if (!hostNodeID) {
        continue;
      }
      const edgeColor = "var(--chart)";
      flowEdges.push({
        id: `edge-runs-on:${guest.guest.id}:${guest.linkedHostID}`,
        source: guest.guestNodeID,
        target: hostNodeID,
        type: "smoothstep",
        label: flowEdgeBadge(
          guest.isAuto ? words.auto : words.manual,
          guest.isAuto
            ? words.autoLink(guest.guest.name, guest.linkedHostName ?? "")
            : words.manualLink(guest.guest.name, guest.linkedHostName ?? ""),
        ),
        labelShowBg: false,
        style: {
          stroke: edgeColor,
          strokeWidth: guest.guestSelected ? 2.5 : 1.5,
          strokeDasharray: guest.isAuto ? "6 4" : undefined,
        },
        markerEnd: { type: MarkerType.ArrowClosed, color: edgeColor },
      });
      continue;
    }

    const missingNodeID = missingNodeIDByTarget.get(guest.mapping.target_asset_id);
    if (!missingNodeID) {
      continue;
    }
    flowEdges.push({
      id: `edge-runs-on-missing:${guest.guest.id}:${guest.mapping.target_asset_id}`,
      source: guest.guestNodeID,
      target: missingNodeID,
      type: "smoothstep",
      label: flowEdgeBadge(words.missing, words.missingLink(guest.mapping.target_asset_id)),
      labelShowBg: false,
      style: {
        stroke: "var(--bad)",
        strokeWidth: guest.guestSelected ? 2.5 : 1.5,
        strokeDasharray: "6 4",
      },
      markerEnd: { type: MarkerType.ArrowClosed, color: "var(--bad)" },
    });
  }

  return { nodes: flowNodes, edges: flowEdges };
}
