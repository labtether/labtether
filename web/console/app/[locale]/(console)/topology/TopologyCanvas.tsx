"use client";

import { buildAssetNodes,buildConnectionEdges,buildZoneNodes } from './topologyCanvasModel';


import {
applyEdgeChanges,
applyNodeChanges,
Background,
BackgroundVariant,
Controls,
MiniMap,
ReactFlow,
ReactFlowProvider,
useReactFlow,
type Connection,
type Edge,
type EdgeMouseHandler,
type Node,
type NodeMouseHandler,
type OnEdgesChange,
type OnNodesChange,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useCallback,useEffect,useMemo,useState } from "react";
import { isInfraHost } from "../../../console/taxonomy";
import { useFastStatus } from "../../../contexts/StatusContext";
import { ContainmentCardNode } from "./ContainmentCard";
import type { RelationshipType,TopologyState,Viewport } from "./topologyCanvasTypes";
import { TopologyContextMenu,type ContextMenuTarget } from "./TopologyContextMenus";
import { buildHierarchy } from "./topologyHierarchy";
import { inferRelationshipType } from "./topologySmartDefaults";
import { useAssetDependencies } from "./useAssetDependencies";
import { ZoneNode } from "./ZoneNode";

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
