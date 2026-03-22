"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  type Node,
  type Edge,
  type Connection,
  applyNodeChanges,
  applyEdgeChanges,
  type OnNodesChange,
  type OnEdgesChange,
  BackgroundVariant,
  ReactFlowProvider,
  useReactFlow,
  type NodeMouseHandler,
  type EdgeMouseHandler,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { ZoneNode } from "./ZoneNode";
import { ContainmentCardNode } from "./ContainmentCard";
import type { ContainmentCardData, ContainmentLayerData } from "./ContainmentCard";
import type { ContainmentChild } from "./ContainmentLayer";
import type { TopologyState, TopologyConnection, RelationshipType, Viewport } from "./topologyCanvasTypes";
import { TopologyContextMenu, type ContextMenuTarget } from "./TopologyContextMenus";
import { inferRelationshipType } from "./topologySmartDefaults";
import type { Asset } from "../../../console/models";
import { useFastStatus } from "../../../contexts/StatusContext";
import { useAssetDependencies } from "./useAssetDependencies";
import { buildHierarchy, formatWorkloadSummary } from "./topologyHierarchy";
import { isInfraHost } from "../../../console/taxonomy";

const nodeTypes = { zone: ZoneNode, asset: ContainmentCardNode };

interface TopologyCanvasProps {
  topology: TopologyState;
  onViewportChange?: (viewport: Viewport) => void;
  onZoneLabelChange?: (zoneId: string, label: string) => void;
  onZoneToggleCollapse?: (zoneId: string) => void;
  onZoneDelete?: (zoneId: string) => void;
  onZoneResize?: (zoneId: string, width: number, height: number) => void;
  onZoneMove?: (zoneId: string, x: number, y: number) => void;
  onAssetSelect?: (assetId: string | null) => void;
  onConnectionSelect?: (connId: string | null) => void;
  onCreateConnection?: (conn: { source_asset_id: string; target_asset_id: string; relationship: RelationshipType }) => void;
  // Context menu callbacks
  onCreateZone?: () => void;
  onConnectTo?: (assetId: string) => void;
  onMoveToZone?: (assetId: string, zoneId: string) => void;
  onRemoveFromZone?: (assetId: string) => void;
  onChangeConnectionType?: (connId: string, type: RelationshipType) => void;
  onDeleteConnection?: (connId: string) => void;
  onResetLayout?: () => void;
}

function buildZoneNodes(
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

/** Group label for workload-type children */
function layerLabel(type: string): string {
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

function buildAssetNodes(
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

function getConnectionColor(relationship: string): string {
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

function buildConnectionEdges(connections: TopologyConnection[], visibleAssetIDs: Set<string>): Edge[] {
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

function TopologyCanvasInner({
  topology,
  onViewportChange,
  onZoneLabelChange,
  onZoneToggleCollapse,
  onZoneDelete,
  onZoneMove,
  onAssetSelect,
  onConnectionSelect,
  onCreateConnection,
  onCreateZone,
  onConnectTo,
  onMoveToZone,
  onRemoveFromZone,
  onChangeConnectionType,
  onDeleteConnection,
  onResetLayout,
}: TopologyCanvasProps) {
  // Fetch asset data from status context for containment card rendering
  const fastStatus = useFastStatus();
  const allAssets = useMemo(() => fastStatus?.assets ?? [], [fastStatus?.assets]);
  const assetsByID = useMemo(() => {
    const map = new Map<string, (typeof allAssets)[number]>();
    for (const a of allAssets) map.set(a.id, a);
    return map;
  }, [allAssets]);
  const assetIDs = useMemo(() => allAssets.map((a) => a.id).sort(), [allAssets]);
  const { dependencies } = useAssetDependencies(assetIDs);
  const hierarchyDependencies = useMemo(() => {
    const merged = [...dependencies];
    const seen = new Set(merged.map((dependency) => dependency.id));
    for (const connection of topology.connections) {
      if (seen.has(connection.id)) {
        continue;
      }
      seen.add(connection.id);
      merged.push({
        id: connection.id,
        source_asset_id: connection.source_asset_id,
        target_asset_id: connection.target_asset_id,
        relationship_type: connection.relationship,
        origin: connection.origin === "user" ? "manual" : "auto",
      });
    }
    return merged;
  }, [dependencies, topology.connections]);

  // Build hierarchy for containment layers
  const hierarchy = useMemo(
    () => buildHierarchy(allAssets, hierarchyDependencies),
    [allAssets, hierarchyDependencies],
  );

  // Context menu + rename state (declared early so zoneNodes memo can reference them)
  const [contextMenu, setContextMenu] = useState<ContextMenuTarget | null>(null);
  const [renamingZoneId, setRenamingZoneId] = useState<string | null>(null);
  const { fitView } = useReactFlow();

  const zoneNodes = useMemo(
    () => buildZoneNodes(
      topology,
      {
        onLabelChange: onZoneLabelChange,
        onToggleCollapse: onZoneToggleCollapse,
        onDelete: onZoneDelete,
        onRenameDone: () => setRenamingZoneId(null),
      },
      renamingZoneId,
      hierarchy.childIDs,
      hierarchy.parentOf,
    ),
    [topology, onZoneLabelChange, onZoneToggleCollapse, onZoneDelete, renamingZoneId, hierarchy.childIDs, hierarchy.parentOf],
  );

  const assetNodes = useMemo(
    () => buildAssetNodes(topology, allAssets, hierarchy, { onSelect: onAssetSelect ?? undefined }),
    [topology, allAssets, hierarchy, onAssetSelect],
  );

  const initialNodes = useMemo(
    () => [...zoneNodes, ...assetNodes],
    [zoneNodes, assetNodes],
  );

  // Track which asset IDs are visible on canvas — mirrors buildAssetNodes filtering
  const visibleAssetIDs = useMemo(() => {
    const memberIDSet = new Set(topology.members.map(m => m.asset_id));
    const ids = new Set<string>();
    for (const m of topology.members) {
      if (!hierarchy.childIDs.has(m.asset_id)) { ids.add(m.asset_id); continue; }
      // Infra hosts (NAS, hypervisor, etc.) stay visible even when nested
      const asset = allAssets.find(a => a.id === m.asset_id);
      if (asset && isInfraHost(asset)) { ids.add(m.asset_id); continue; }
      const parentID = hierarchy.parentOf.get(m.asset_id);
      if (!parentID || !memberIDSet.has(parentID)) ids.add(m.asset_id);
    }
    return ids;
  }, [topology.members, hierarchy.childIDs, hierarchy.parentOf, allAssets]);

  const connectionEdges = useMemo(
    () => buildConnectionEdges(topology.connections, visibleAssetIDs),
    [topology.connections, visibleAssetIDs],
  );

  // Use controlled mode: nodes/edges come directly from memos, updated via applyNodeChanges/applyEdgeChanges.
  // This avoids the stale-state bug where useNodesState ignores memo updates after initial render.
  const [nodes, setNodes] = useState(initialNodes);
  const [edges, setEdges] = useState(connectionEdges);

  // Sync from upstream data (topology + status context changes)
  useEffect(() => { setNodes(initialNodes); }, [initialNodes]);
  useEffect(() => { setEdges(connectionEdges); }, [connectionEdges]);

  const onNodesChange: OnNodesChange = useCallback(
    (changes) => setNodes((nds) => applyNodeChanges(changes, nds)),
    [],
  );
  const onEdgesChange: OnEdgesChange = useCallback(
    (changes) => setEdges((eds) => applyEdgeChanges(changes, eds)),
    [],
  );

  const handleConnect = useCallback(
    (connection: Connection) => {
      if (!connection.source || !connection.target) return;
      const sourceAssetId = connection.source.replace("asset-", "");
      const targetAssetId = connection.target.replace("asset-", "");
      const sourceAsset = assetsByID.get(sourceAssetId);
      const targetAsset = assetsByID.get(targetAssetId);
      const relationship = inferRelationshipType(
        sourceAsset?.type || "",
        targetAsset?.type || "",
      );
      onCreateConnection?.({ source_asset_id: sourceAssetId, target_asset_id: targetAssetId, relationship });
    },
    [assetsByID, onCreateConnection],
  );

  const handleEdgeClick = useCallback(
    (_: unknown, edge: Edge) => {
      if (edge.id.startsWith("conn-")) {
        onConnectionSelect?.(edge.id.replace("conn-", ""));
      }
    },
    [onConnectionSelect],
  );

  const handleNodeDragStop = useCallback(
    (_: unknown, node: Node) => {
      if (node.id.startsWith("zone-")) {
        const zoneId = node.id.replace("zone-", "");
        onZoneMove?.(zoneId, node.position.x, node.position.y);
      }
    },
    [onZoneMove],
  );

  const handleNodeClick = useCallback(
    (_: unknown, node: Node) => {
      if (node.id.startsWith("asset-")) {
        onAssetSelect?.(node.id.replace("asset-", ""));
      }
    },
    [onAssetSelect],
  );

  const handlePaneClick = useCallback(() => {
    onAssetSelect?.(null);
    setContextMenu(null);
  }, [onAssetSelect]);

  // (contextMenu, renamingZoneId, fitView declared above zoneNodes memo)

  const handlePaneContextMenu = useCallback(
    (e: React.MouseEvent | MouseEvent) => {
      e.preventDefault();
      const clientX = "clientX" in e ? e.clientX : 0;
      const clientY = "clientY" in e ? e.clientY : 0;
      setContextMenu({ type: "canvas", x: clientX, y: clientY });
    },
    [],
  );

  const handleNodeContextMenu: NodeMouseHandler = useCallback(
    (e, node) => {
      e.preventDefault();
      if (node.id.startsWith("zone-")) {
        const zoneId = node.id.replace("zone-", "");
        const zone = topology.zones.find((z) => z.id === zoneId);
        setContextMenu({ type: "zone", zoneId, label: zone?.label ?? zoneId, x: e.clientX, y: e.clientY });
      } else if (node.id.startsWith("asset-")) {
        const assetId = node.id.replace("asset-", "");
        setContextMenu({ type: "asset", assetId, x: e.clientX, y: e.clientY });
      }
    },
    [topology.zones],
  );

  const handleEdgeContextMenu: EdgeMouseHandler = useCallback(
    (e, edge) => {
      e.preventDefault();
      if (edge.id.startsWith("conn-")) {
        const connectionId = edge.id.replace("conn-", "");
        setContextMenu({ type: "connection", connectionId, x: e.clientX, y: e.clientY });
      }
    },
    [],
  );

  const handleFitView = useCallback(() => {
    fitView({ padding: 0.15, duration: 300 });
  }, [fitView]);

  const handleAutoLayout = useCallback(() => {
    // Get all top-level zone nodes (no parent), sorted by area descending
    const zoneEntries = topology.zones
      .filter((z) => !z.parent_zone_id)
      .map((z) => ({ zone: z, area: z.size.width * z.size.height }))
      .sort((a, b) => b.area - a.area);

    const COLS = 3;
    const GAP = 50;

    zoneEntries.forEach(({ zone }, i) => {
      const col = i % COLS;
      const row = Math.floor(i / COLS);

      // Compute x offset: sum widths + gaps of previous zones in this row
      let x = 0;
      for (let c = 0; c < col; c++) {
        const idx = row * COLS + c;
        if (idx < zoneEntries.length) {
          x += zoneEntries[idx].zone.size.width + GAP;
        }
      }

      // Compute y offset: sum heights + gaps of tallest zone in each previous row
      let y = 0;
      for (let r = 0; r < row; r++) {
        let rowMaxHeight = 0;
        for (let c = 0; c < COLS; c++) {
          const idx = r * COLS + c;
          if (idx < zoneEntries.length) {
            rowMaxHeight = Math.max(rowMaxHeight, zoneEntries[idx].zone.size.height);
          }
        }
        y += rowMaxHeight + GAP;
      }

      onZoneMove?.(zone.id, x, y);
    });

    // Fit view after layout settles
    setTimeout(() => fitView({ padding: 0.15, duration: 300 }), 50);
  }, [topology.zones, onZoneMove, fitView]);

  return (
    <div className="h-full w-full" onContextMenu={(e) => e.preventDefault()}>
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={handleConnect}
      onNodeDragStop={handleNodeDragStop}
      onNodeClick={handleNodeClick}
      onEdgeClick={handleEdgeClick}
      onPaneClick={handlePaneClick}
      onPaneContextMenu={handlePaneContextMenu}
      onNodeContextMenu={handleNodeContextMenu}
      onEdgeContextMenu={handleEdgeContextMenu}
      onMoveEnd={(_, viewport) => onViewportChange?.({ x: viewport.x, y: viewport.y, zoom: viewport.zoom })}
      nodeTypes={nodeTypes}
      defaultViewport={topology.viewport}
      fitView={!topology.viewport || (topology.viewport.x === 0 && topology.viewport.y === 0 && topology.viewport.zoom === 1)}
      minZoom={0.1}
      maxZoom={3}
      nodesDraggable
      nodesConnectable
      selectionOnDrag
      selectNodesOnDrag={false}
      proOptions={{ hideAttribution: true }}
    >
      <Background variant={BackgroundVariant.Dots} gap={24} size={1} color="var(--surface)" />
      <Controls
        showInteractive={false}
        className="!bg-[var(--panel-glass)] !border-[var(--panel-border)] !shadow-none !rounded-[var(--radius-md)] [&>button]:!bg-transparent [&>button]:!border-[var(--line)] [&>button]:!text-[var(--muted)] [&>button:hover]:!bg-[var(--hover)]"
        style={{ backdropFilter: "blur(var(--blur-sm))", WebkitBackdropFilter: "blur(var(--blur-sm))" }}
      />
      <MiniMap
        style={{
          background: "var(--panel-glass)",
          border: "1px solid var(--panel-border)",
          borderRadius: "var(--radius-md)",
          backdropFilter: "blur(8px)",
          WebkitBackdropFilter: "blur(8px)",
        }}
        maskColor="rgba(0,0,0,0.6)"
      />
    </ReactFlow>
    <TopologyContextMenu
      target={contextMenu}
      zones={topology.zones}
      onClose={() => setContextMenu(null)}
      onCreateZone={onCreateZone}
      onFitView={handleFitView}
      onAutoLayout={handleAutoLayout}
      onRenameZone={onZoneLabelChange ? (zoneId) => {
        setRenamingZoneId(zoneId);
      } : undefined}
      onDeleteZone={onZoneDelete}
      onToggleCollapse={onZoneToggleCollapse}
      onConnectTo={onConnectTo}
      onMoveToZone={onMoveToZone}
      onRemoveFromZone={onRemoveFromZone}
      onChangeConnectionType={onChangeConnectionType}
      onDeleteConnection={onDeleteConnection}
      onResetLayout={onResetLayout}
    />
    </div>
  );
}

export default function TopologyCanvas(props: TopologyCanvasProps) {
  return (
    <ReactFlowProvider>
      <TopologyCanvasInner {...props} />
    </ReactFlowProvider>
  );
}
