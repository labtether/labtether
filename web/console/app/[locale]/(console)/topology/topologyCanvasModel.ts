"use client";

import {
type Edge,
type Node
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { Asset } from "../../../console/models";
import { isInfraHost } from "../../../console/taxonomy";
import type { ContainmentCardData,ContainmentLayerData } from "./ContainmentCard";
import type { ContainmentChild } from "./ContainmentLayer";
import type { TopologyConnection,TopologyState } from "./topologyCanvasTypes";
import { buildHierarchy,formatWorkloadSummary } from "./topologyHierarchy";

export function buildZoneNodes(
  topology: TopologyState,
  callbacks: {
    onLabelChange?: (zoneId: string, label: string) => void;
    onToggleCollapse?: (zoneId: string) => void;
    onDelete?: (zoneId: string) => void;
    onRenameDone?: (zoneId: string) => void;
  },
  renamingZoneId?: string | null,
  childIDs?: Set<string>,
  parentOf?: Map<string, string>,
): Node[] {
  // Count only visible members per zone. A child is hidden only if its parent
  // is also a member AND the child is not itself an infra host.
  // NOTE: We don't have Asset objects here (only IDs), so we can't call isInfraHost.
  // Instead we use a simple heuristic: infra host types from childIDs are rare
  // in practice, and the count will be close enough. The exact filtering happens
  // in buildAssetNodes which has access to the full asset data.
  const memberIDSet = new Set(topology.members.map(m => m.asset_id));
  const memberCountByZone = new Map<string, number>();
  for (const m of topology.members) {
    if (childIDs?.has(m.asset_id)) {
      const pid = parentOf?.get(m.asset_id);
      if (pid && memberIDSet.has(pid)) continue; // parent visible, skip child
    }
    memberCountByZone.set(m.zone_id, (memberCountByZone.get(m.zone_id) || 0) + 1);
  }
  const subZoneCountByParent = new Map<string, number>();
  for (const z of topology.zones) {
    if (z.parent_zone_id) {
      subZoneCountByParent.set(z.parent_zone_id, (subZoneCountByParent.get(z.parent_zone_id) || 0) + 1);
    }
  }

  // Auto-sizing constants for zones still at seed-default dimensions
  const CARD_W = 300; // ContainmentCard width (280) + 20px gap
  const CARD_H = 56;
  const PADDING = 40;
  const HEADER = 36;

  return topology.zones.map((z) => {
    const memberCount = memberCountByZone.get(z.id) || 0;

    // Always compute the right size from visible member count. The backend
    // can't know what the frontend will filter (children hidden when parent
    // is visible), so we override seed-generated sizes too.
    const cols = Math.min(memberCount, 2);
    const rows = Math.max(1, Math.ceil(memberCount / 2));
    const autoWidth = cols * CARD_W + PADDING;
    const autoHeight = HEADER + rows * CARD_H + PADDING;
    const size = { width: Math.max(300, autoWidth), height: Math.max(120, autoHeight) };

    return {
      id: `zone-${z.id}`,
      type: "zone" as const,
      position: { x: z.position.x, y: z.position.y },
      data: {
        zoneId: z.id,
        label: z.label,
        color: z.color,
        collapsed: z.collapsed,
        assetCount: memberCount,
        subZoneCount: subZoneCountByParent.get(z.id) || 0,
        renaming: renamingZoneId === z.id,
        onLabelChange: callbacks.onLabelChange,
        onToggleCollapse: callbacks.onToggleCollapse,
        onDelete: callbacks.onDelete,
        onRenameDone: callbacks.onRenameDone,
      },
      style: { width: size.width, height: size.height },
      // Nest inside parent zone
      ...(z.parent_zone_id ? { parentId: `zone-${z.parent_zone_id}` } : {}),
      dragHandle: ".cursor-grab",
    };
  });
}

export function layerLabel(type: string): string {
  const t = type.trim().toLowerCase();
  if (t === "vm") return "Virtual Machines";
  if (t === "container" || t === "docker-container") return "Containers";
  if (t === "pod") return "Pods";
  if (t === "service" || t === "ha-entity") return "Services";
  if (t === "stack" || t === "compose-stack" || t === "deployment") return "Stacks";
  if (t === "storage-pool" || t === "datastore") return "Storage Pools";
  if (t === "dataset" || t === "disk" || t === "share-smb" || t === "share-nfs" || t === "snapshot") return "Datasets";
  return "Other";
}

export function buildAssetNodes(
  topology: TopologyState,
  assets: Asset[],
  hierarchy: ReturnType<typeof buildHierarchy>,
  callbacks: { onSelect?: (assetId: string) => void },
): Node[] {
  const assetByID = new Map<string, Asset>();
  for (const a of assets) assetByID.set(a.id, a);

  // Build a lookup: hostID -> HierarchyEntry for containment data
  const entryByHostID = new Map<string, (typeof hierarchy.entries)[number]>();
  for (const entry of hierarchy.entries) {
    entryByHostID.set(entry.host.id, entry);
  }

  // Only hide a child if:
  // 1. Its parent is also a zone member (visible on canvas), AND
  // 2. The child is NOT itself an infra host (hosts with their own
  //    containment hierarchy must remain visible even when nested).
  const memberIDSet = new Set(topology.members.map(m => m.asset_id));
  const topLevelMembers = topology.members.filter(m => {
    if (!hierarchy.childIDs.has(m.asset_id)) return true;
    const asset = assetByID.get(m.asset_id);
    if (asset && isInfraHost(asset)) return true; // infra hosts always visible
    const parentID = hierarchy.parentOf.get(m.asset_id);
    return !parentID || !memberIDSet.has(parentID);
  });

  return topLevelMembers.map((m) => {
    const asset = assetByID.get(m.asset_id);
    const name = asset?.name ?? m.asset_id;
    const type = asset?.type ?? "unknown";
    const source = asset?.source ?? "unknown";
    const status = asset?.status ?? "unknown";

    // Collect unique sources for this asset
    const sources = [source];

    // Build containment layers from hierarchy entry
    const layers: ContainmentLayerData[] = [];
    const entry = entryByHostID.get(m.asset_id);
    let summaryBadge = "";
    let hasChildren = false;

    if (entry) {
      hasChildren = entry.deviceChildren.length > 0 || entry.workloadChildren.length > 0;
      summaryBadge = formatWorkloadSummary(entry.workloads);
      if (summaryBadge === "no workloads" && entry.deviceChildren.length === 0) {
        summaryBadge = "";
      }

      // Group all children (device + workload) by source+label
      const allChildren = [...entry.deviceChildren, ...entry.workloadChildren];
      const layerMap = new Map<string, { label: string; source: string; children: ContainmentChild[] }>();

      for (const child of allChildren) {
        const lbl = layerLabel(child.type);
        const key = `${child.source}:${lbl}`;
        let layer = layerMap.get(key);
        if (!layer) {
          layer = { label: lbl, source: child.source, children: [] };
          layerMap.set(key, layer);
        }
        layer.children.push({
          id: child.id,
          name: child.name,
          type: child.type,
          source: child.source,
          status: child.status,
          port: child.metadata?.port,
        });

        // Track unique sources for multi-source badge
        if (!sources.includes(child.source)) {
          sources.push(child.source);
        }
      }

      layers.push(...layerMap.values());
    }

    const cardData: ContainmentCardData = {
      assetId: m.asset_id,
      name,
      type,
      sources,
      status,
      summaryBadge,
      layers,
      hasChildren,
      onSelect: callbacks.onSelect,
    };

    return {
      id: `asset-${m.asset_id}`,
      type: "asset" as const,
      position: { x: m.position.x, y: m.position.y },
      parentId: `zone-${m.zone_id}`,
      data: cardData,
    };
  });
}

export function getConnectionColor(relationship: string): string {
  switch (relationship) {
    case "runs_on":
    case "hosted_on":
      return "var(--ok)";       // neon green
    case "depends_on":
      return "#f97316";          // orange
    case "provides_to":
      return "var(--accent)";   // neon rose
    default:
      return "var(--muted)";
  }
}

export function buildConnectionEdges(connections: TopologyConnection[], visibleAssetIDs: Set<string>): Edge[] {
  return connections
    .filter(conn => visibleAssetIDs.has(conn.source_asset_id) && visibleAssetIDs.has(conn.target_asset_id))
    .map((conn) => {
      const color = getConnectionColor(conn.relationship);
      return {
        id: `conn-${conn.id}`,
        source: `asset-${conn.source_asset_id}`,
        target: `asset-${conn.target_asset_id}`,
        type: "default",
        animated: conn.relationship === "runs_on",
        style: {
          stroke: color,
          strokeWidth: 2,
          strokeDasharray: conn.origin === "discovered" ? "6 4" : undefined,
          opacity: conn.origin === "discovered" ? 0.5 : 0.6,
          filter: `drop-shadow(0 0 3px ${color})`,
          transition: `opacity var(--dur-fast) ease`,
        },
        data: { connectionId: conn.id, relationship: conn.relationship, origin: conn.origin },
      };
    });
}
