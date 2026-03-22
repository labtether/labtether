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
  guestStatusColor,
  sourceBadgeLabel,
} from "./clusterTopologyUtils";
import type {
  ClusterPlacement,
  GuestPlacement,
} from "./clusterTopologyFlowPlacements";

export type ClusterTopologyNodeData = {
  label: ReactNode;
  assetID?: string;
};

function statusStroke(status?: string): string {
  const normalized = (status ?? "").trim().toLowerCase();
  if (normalized === "running" || normalized === "online" || normalized === "active") {
    return "var(--ok)";
  }
  if (normalized === "error" || normalized === "failed") {
    return "var(--bad)";
  }
  if (normalized === "stopped" || normalized === "offline" || normalized === "paused") {
    return "rgba(148, 163, 184, 0.65)";
  }
  return "rgba(255, 255, 255, 0.28)";
}

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
          <div className="space-y-1 text-left">
            <div className="flex items-center gap-1.5">
              <span className={`inline-block h-2 w-2 rounded-full ${cluster.nodeOnline ? "bg-[var(--ok)]" : "bg-[var(--bad)]"}`} />
              <span className="text-xs font-semibold">{cluster.nodeName}</span>
              {cluster.nodeLocal ? (
                <span className="rounded bg-[var(--hover)] px-1 py-0.5 text-[10px] text-[var(--muted)]">local</span>
              ) : null}
            </div>
            <p className="text-[10px] text-[var(--muted)]">{cluster.nodeIP}</p>
            <p className="text-[10px] text-[var(--muted)]">
              {cluster.guestCount} guest{cluster.guestCount !== 1 ? "s" : ""}
            </p>
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
          <div className="space-y-1 text-left">
            <p className="text-xs font-semibold">{guest.guest.name}</p>
            <div className="flex flex-wrap items-center gap-1">
              <span className="rounded bg-[var(--hover)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">
                {friendlyTypeLabel(guest.guest.type)}
              </span>
              {guest.guestStatus ? (
                <span className={`text-[10px] ${guestStatusColor(guest.guestStatus)}`}>{guest.guestStatus}</span>
              ) : null}
              {guest.isAuto ? (
                <span className="rounded bg-[var(--ok-glow)] px-1.5 py-0.5 text-[10px] text-[var(--ok)]">auto</span>
              ) : null}
            </div>
            {guest.linkedHostName ? (
              <p className="text-[10px] text-[var(--accent-text)]">linked to {guest.linkedHostName}</p>
            ) : null}
          </div>
        ),
        assetID: guest.guest.id,
      },
      style: {
        ...baseNodeStyle,
        borderColor: guest.guestSelected ? "rgba(250, 204, 21, 0.9)" : statusStroke(guest.guestStatus),
        width: guestNodeWidth,
        boxShadow: guest.guestSelected
          ? "0 0 0 2px rgba(250, 204, 21, 0.4), 0 10px 24px rgba(0, 0, 0, 0.28)"
          : baseNodeStyle.boxShadow,
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
          <div className="space-y-1 text-left">
            <p className="text-xs font-semibold">{host.name}</p>
            <div className="flex flex-wrap items-center gap-1">
              <span className="rounded bg-[var(--accent-subtle)] px-1.5 py-0.5 text-[10px] text-[var(--accent-text)]">
                {sourceBadgeLabel(host.source)}
              </span>
              {linkedCount > 0 ? (
                <span className="rounded bg-[var(--hover)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">
                  {linkedCount} link{linkedCount !== 1 ? "s" : ""}
                </span>
              ) : null}
            </div>
          </div>
        ),
        assetID: host.id,
      },
      style: {
        ...baseNodeStyle,
        borderColor: "rgba(14, 165, 233, 0.5)",
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
          <div className="space-y-1 text-left">
            <p className="text-xs font-semibold text-[var(--bad)]">Missing target</p>
            <p className="text-[10px] text-[var(--muted)] break-all">{targetID}</p>
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
        stroke: guest.guestSelected
          ? "rgba(250, 204, 21, 0.78)"
          : (guest.nodeOnline ? "var(--ok-glow)" : "var(--bad-glow)"),
        strokeWidth: guest.guestSelected ? 2.6 : 1.5,
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
      const edgeColor = guest.isAuto ? "rgba(16, 185, 129, 0.8)" : "rgba(14, 165, 233, 0.85)";
      flowEdges.push({
        id: `edge-runs-on:${guest.guest.id}:${guest.linkedHostID}`,
        source: guest.guestNodeID,
        target: hostNodeID,
        type: "smoothstep",
        animated: guest.isAuto,
        label: flowEdgeBadge(
          guest.isAuto ? "auto" : "manual",
          `${guest.isAuto ? "Auto" : "Manual"} link: ${guest.guest.name} -> ${guest.linkedHostName ?? "host"}`,
          guest.isAuto ? "success" : "info",
        ),
        labelShowBg: false,
        style: {
          stroke: edgeColor,
          strokeWidth: guest.guestSelected ? 3 : 2,
          strokeDasharray: guest.isAuto ? "7 4" : undefined,
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
      label: flowEdgeBadge(
        "missing",
        `Missing target asset: ${guest.mapping.target_asset_id}`,
        "danger",
      ),
      labelShowBg: false,
      style: {
        stroke: guest.guestSelected ? "var(--warn)" : "var(--bad)",
        strokeWidth: guest.guestSelected ? 3 : 2,
        strokeDasharray: "6 4",
      },
      markerEnd: { type: MarkerType.ArrowClosed, color: "var(--bad)" },
    });
  }

  return { nodes: flowNodes, edges: flowEdges };
}
