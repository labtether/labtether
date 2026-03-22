"use client";

import { useCallback, useDeferredValue, useEffect, useMemo, type CSSProperties } from "react";
import { useRouter } from "../../i18n/navigation";
import {
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
  Handle,
  Position,
} from "@xyflow/react";
import { useFastStatus } from "../contexts/StatusContext";
import type { Asset, TelemetryOverviewAsset } from "../console/models";
import { isHiddenAsset, isInfraHost } from "../console/taxonomy";
import {
  assetFreshness,
  relationshipPriority,
} from "../[locale]/(console)/topology/topologyUtils";
import { useAssetDependencies, type AssetDependency } from "../[locale]/(console)/topology/useAssetDependencies";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type StatusTier = "ok" | "warn" | "bad" | "offline";

type HeroNodeData = {
  label: string;
  status: StatusTier;
  cpuPercent?: number;
  memPercent?: number;
  assetId: string;
  isHub?: boolean;
};

type HeroEdgeData = {
  alive: boolean;
};

type HeroEdgeInput = {
  sourceID: string;
  targetID: string;
  relationshipType: string;
  inferred: boolean;
};

type Point = { x: number; y: number };
const HERO_MAX_NODES = 24;

// ---------------------------------------------------------------------------
// Status helpers
// ---------------------------------------------------------------------------

function freshnessToTier(asset: Asset): StatusTier {
  const f = assetFreshness(asset);
  if (f === "online") return "ok";
  if (f === "unresponsive") return "warn";
  if (f === "offline") return "bad";
  return "offline";
}

const STATUS_COLORS: Record<StatusTier, { ring: string; glow: string; glowVar: string; rgb: string }> = {
  ok: {
    ring: "var(--ok)",
    glow: "var(--ok-glow)",
    glowVar: "0 0 14px 3px var(--ok-glow), 0 0 4px 1px var(--ok-glow)",
    rgb: "var(--ok)",
  },
  warn: {
    ring: "var(--warn)",
    glow: "var(--warn-glow)",
    glowVar: "0 0 14px 3px var(--warn-glow), 0 0 4px 1px var(--warn-glow)",
    rgb: "var(--warn)",
  },
  bad: {
    ring: "var(--bad)",
    glow: "var(--bad-glow)",
    glowVar: "0 0 14px 3px var(--bad-glow), 0 0 4px 1px var(--bad-glow)",
    rgb: "var(--bad)",
  },
  offline: {
    ring: "#71717a",
    glow: "transparent",
    glowVar: "none",
    rgb: "#71717a",
  },
};

// ---------------------------------------------------------------------------
// Custom node component
// ---------------------------------------------------------------------------

function TopologyHeroNode({ data }: NodeProps<Node<HeroNodeData>>) {
  const sc = STATUS_COLORS[data.status];
  const isOffline = data.status === "offline";
  const isHub = data.isHub ?? false;
  const size = isHub ? 40 : 26;
  const r = size / 2;
  const outerR = r + 6;
  const svgSize = outerR * 2 + 4;
  const center = svgSize / 2;

  const tooltipLines: string[] = [data.label];
  if (data.cpuPercent !== undefined) tooltipLines.push(`CPU: ${data.cpuPercent.toFixed(0)}%`);
  if (data.memPercent !== undefined) tooltipLines.push(`Mem: ${data.memPercent.toFixed(0)}%`);
  if (isOffline) tooltipLines.push("Offline");

  return (
    <div
      className="topology-hero-node flex flex-col items-center"
      title={tooltipLines.join("\n")}
      style={{ width: svgSize, marginBottom: 2 }}
    >
      <Handle id="top-in" type="target" position={Position.Top} className="!bg-transparent !border-none !w-0 !h-0" />
      <Handle id="right-in" type="target" position={Position.Right} className="!bg-transparent !border-none !w-0 !h-0" />
      <Handle id="bottom-in" type="target" position={Position.Bottom} className="!bg-transparent !border-none !w-0 !h-0" />
      <Handle id="left-in" type="target" position={Position.Left} className="!bg-transparent !border-none !w-0 !h-0" />

      <svg width={svgSize} height={svgSize} style={{ overflow: "visible", opacity: isOffline ? 0.5 : 1 }}>
        {/* Outer glow ring */}
        <circle cx={center} cy={center} r={outerR} fill={sc.glow} opacity={0.15} />
        {/* Node ring */}
        <circle
          cx={center} cy={center} r={r}
          fill={sc.glow}
          stroke={sc.ring}
          strokeWidth={isHub ? 2 : 1.5}
          opacity={isHub ? 1 : 0.7}
        />
        {/* Hub pulsing ring */}
        {isHub && (
          <circle
            cx={center} cy={center} r={r + 3}
            fill="none"
            stroke={sc.ring}
            strokeWidth={0.5}
            opacity={0.3}
            style={{ animation: "glow-breathe 3s ease-in-out infinite" }}
          />
        )}
        {/* Center dot */}
        <circle cx={center} cy={center} r={isHub ? 4 : 3} fill={sc.ring} />
        {/* Center dot glow */}
        <circle
          cx={center} cy={center} r={isHub ? 6 : 4}
          fill={sc.ring}
          opacity={0.15}
          style={{ filter: "blur(2px)" }}
        />
      </svg>

      <span
        className="max-w-[160px] truncate text-center leading-tight font-mono"
        style={{
          fontSize: 9,
          color: "var(--muted)",
          opacity: isOffline ? 0.45 : 0.7,
          fontWeight: 500,
          marginTop: 2,
        }}
      >
        {data.label}
      </span>

      <Handle id="top-out" type="source" position={Position.Top} className="!bg-transparent !border-none !w-0 !h-0" />
      <Handle id="right-out" type="source" position={Position.Right} className="!bg-transparent !border-none !w-0 !h-0" />
      <Handle id="bottom-out" type="source" position={Position.Bottom} className="!bg-transparent !border-none !w-0 !h-0" />
      <Handle id="left-out" type="source" position={Position.Left} className="!bg-transparent !border-none !w-0 !h-0" />
    </div>
  );
}

const nodeTypes = { heroNode: TopologyHeroNode };

// ---------------------------------------------------------------------------
// Layout and edge helpers
// ---------------------------------------------------------------------------

function layoutNodes(assets: Asset[]): { x: number; y: number }[] {
  const count = assets.length;
  if (count === 0) return [];
  if (count === 1) return [{ x: 200, y: 110 }];

  // Arrange nodes in an ellipse
  const cx = 200;
  const cy = 110;
  const rx = 160;
  const ry = 80;
  const positions: { x: number; y: number }[] = [];

  for (let i = 0; i < count; i++) {
    const angle = (2 * Math.PI * i) / count - Math.PI / 2;
    positions.push({
      x: cx + rx * Math.cos(angle),
      y: cy + ry * Math.sin(angle),
    });
  }

  return positions;
}

function pointDistance(left: Point, right: Point): number {
  return Math.hypot(right.x - left.x, right.y - left.y);
}

function pickDirection(from: Point, to: Point): "left" | "right" | "top" | "bottom" {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  if (Math.abs(dx) >= Math.abs(dy)) {
    return dx >= 0 ? "right" : "left";
  }
  return dy >= 0 ? "bottom" : "top";
}

function oppositeDirection(direction: "left" | "right" | "top" | "bottom"): "left" | "right" | "top" | "bottom" {
  if (direction === "left") return "right";
  if (direction === "right") return "left";
  if (direction === "top") return "bottom";
  return "top";
}

function resolveEdgeHandles(sourcePos: Point, targetPos: Point): { sourceHandle: string; targetHandle: string } {
  const sourceDirection = pickDirection(sourcePos, targetPos);
  const targetDirection = oppositeDirection(sourceDirection);
  return {
    sourceHandle: `${sourceDirection}-out`,
    targetHandle: `${targetDirection}-in`,
  };
}

function buildDependencyEdgeInputs(
  selectedByID: Map<string, Asset>,
  positionsByID: Map<string, Point>,
  dependencies: AssetDependency[],
): HeroEdgeInput[] {
  const ranked: Array<HeroEdgeInput & { priority: number; distance: number }> = [];
  const seen = new Set<string>();

  for (const dependency of dependencies) {
    const sourceID = dependency.source_asset_id;
    const targetID = dependency.target_asset_id;
    if (sourceID === targetID) continue;
    if (!selectedByID.has(sourceID) || !selectedByID.has(targetID)) continue;

    const relationshipType = dependency.relationship_type.trim().toLowerCase();
    if (!relationshipType) continue;

    const dedupeKey = `${sourceID}->${targetID}:${relationshipType}`;
    if (seen.has(dedupeKey)) continue;
    seen.add(dedupeKey);

    const sourcePos = positionsByID.get(sourceID);
    const targetPos = positionsByID.get(targetID);
    const distance = sourcePos && targetPos ? pointDistance(sourcePos, targetPos) : Number.MAX_SAFE_INTEGER;

    ranked.push({
      sourceID,
      targetID,
      relationshipType,
      inferred: false,
      priority: relationshipPriority(relationshipType, false),
      distance,
    });
  }

  if (ranked.length === 0) return [];

  const maxEdges = Math.max(2, Math.min(18, selectedByID.size + 4));
  ranked.sort((left, right) => left.priority - right.priority || left.distance - right.distance);
  return ranked.slice(0, maxEdges).map(({ priority: _priority, distance: _distance, ...edge }) => edge);
}

function buildMstEdgeInputs(selected: Asset[], positionsByID: Map<string, Point>): HeroEdgeInput[] {
  if (selected.length < 2) return [];

  const ids = selected.map((asset) => asset.id);
  const candidates: Array<{ leftID: string; rightID: string; distance: number }> = [];
  for (let left = 0; left < ids.length - 1; left += 1) {
    for (let right = left + 1; right < ids.length; right += 1) {
      const leftID = ids[left];
      const rightID = ids[right];
      const leftPos = positionsByID.get(leftID);
      const rightPos = positionsByID.get(rightID);
      if (!leftPos || !rightPos) continue;
      candidates.push({
        leftID,
        rightID,
        distance: pointDistance(leftPos, rightPos),
      });
    }
  }
  candidates.sort((left, right) => left.distance - right.distance);

  const parent = new Map(ids.map((id) => [id, id]));
  const rank = new Map(ids.map((id) => [id, 0]));

  const find = (id: string): string => {
    const nodeParent = parent.get(id) ?? id;
    if (nodeParent === id) return id;
    const root = find(nodeParent);
    parent.set(id, root);
    return root;
  };

  const union = (leftID: string, rightID: string): boolean => {
    const leftRoot = find(leftID);
    const rightRoot = find(rightID);
    if (leftRoot === rightRoot) return false;

    const leftRank = rank.get(leftRoot) ?? 0;
    const rightRank = rank.get(rightRoot) ?? 0;
    if (leftRank < rightRank) {
      parent.set(leftRoot, rightRoot);
      return true;
    }
    if (leftRank > rightRank) {
      parent.set(rightRoot, leftRoot);
      return true;
    }
    parent.set(rightRoot, leftRoot);
    rank.set(leftRoot, leftRank + 1);
    return true;
  };

  const edges: HeroEdgeInput[] = [];
  for (const candidate of candidates) {
    if (edges.length >= selected.length - 1) break;
    if (!union(candidate.leftID, candidate.rightID)) continue;

    const leftPos = positionsByID.get(candidate.leftID);
    const rightPos = positionsByID.get(candidate.rightID);
    if (!leftPos || !rightPos) continue;

    const useForwardDirection = leftPos.x < rightPos.x || (leftPos.x === rightPos.x && leftPos.y <= rightPos.y);
    edges.push({
      sourceID: useForwardDirection ? candidate.leftID : candidate.rightID,
      targetID: useForwardDirection ? candidate.rightID : candidate.leftID,
      relationshipType: "connected_to",
      inferred: true,
    });
  }

  return edges;
}

// ---------------------------------------------------------------------------
// Graph builder
// ---------------------------------------------------------------------------

function buildHeroGraph(
  visibleAssets: Asset[],
  telemetry: TelemetryOverviewAsset[],
  dependencies: AssetDependency[],
): { nodes: Node[]; edges: Edge[] } {
  if (visibleAssets.length === 0) {
    return { nodes: [], edges: [] };
  }

  const sorted = [...visibleAssets].sort((a, b) => a.name.localeCompare(b.name));
  const selected = sorted.slice(0, HERO_MAX_NODES);
  const selectedByID = new Map<string, Asset>(selected.map((asset) => [asset.id, asset]));

  // Build telemetry lookup
  const telemetryByID = new Map<string, TelemetryOverviewAsset>();
  for (const t of telemetry) {
    telemetryByID.set(t.asset_id, t);
  }

  // Layout
  const positions = layoutNodes(selected);
  const positionsByID = new Map<string, Point>();
  for (let index = 0; index < selected.length; index += 1) {
    const asset = selected[index];
    const point = positions[index];
    if (!point) continue;
    positionsByID.set(asset.id, point);
  }

  // Determine hub nodes — nodes that have the most incoming dependencies
  const incomingCount = new Map<string, number>();
  for (const dep of dependencies) {
    if (selectedByID.has(dep.target_asset_id)) {
      incomingCount.set(dep.target_asset_id, (incomingCount.get(dep.target_asset_id) ?? 0) + 1);
    }
  }
  const maxIncoming = Math.max(0, ...incomingCount.values());
  const hubThreshold = Math.max(2, Math.floor(maxIncoming * 0.7));

  const nodes: Node[] = selected.map((asset, i) => {
    const tel = telemetryByID.get(asset.id);
    const position = positions[i] ?? { x: 200, y: 110 };
    return {
      id: asset.id,
      type: "heroNode",
      position,
      draggable: false,
      connectable: false,
      selectable: false,
      data: {
        label: asset.name,
        status: freshnessToTier(asset),
        cpuPercent: tel?.metrics.cpu_used_percent,
        memPercent: tel?.metrics.memory_used_percent,
        assetId: asset.id,
        isHub: (incomingCount.get(asset.id) ?? 0) >= hubThreshold,
      },
    };
  });

  const dependencyEdges = buildDependencyEdgeInputs(selectedByID, positionsByID, dependencies);
  const edgeInputs = dependencyEdges.length > 0
    ? dependencyEdges
    : buildMstEdgeInputs(selected, positionsByID);

  const edges: Edge[] = [];
  for (let index = 0; index < edgeInputs.length; index += 1) {
    const edgeInput = edgeInputs[index];
    const source = selectedByID.get(edgeInput.sourceID);
    const target = selectedByID.get(edgeInput.targetID);
    const sourcePos = positionsByID.get(edgeInput.sourceID);
    const targetPos = positionsByID.get(edgeInput.targetID);
    if (!source || !target || !sourcePos || !targetPos) continue;

    const sourceTier = freshnessToTier(source);
    const targetTier = freshnessToTier(target);
    const alive = sourceTier !== "bad" && sourceTier !== "offline"
      && targetTier !== "bad" && targetTier !== "offline";
    const { sourceHandle, targetHandle } = resolveEdgeHandles(sourcePos, targetPos);
    const sc_source = STATUS_COLORS[sourceTier];

    edges.push({
      id: `hero-${edgeInput.sourceID}-${edgeInput.targetID}-${edgeInput.relationshipType}-${index}`,
      source: edgeInput.sourceID,
      target: edgeInput.targetID,
      sourceHandle,
      targetHandle,
      type: "straight",
      data: { alive },
      style: {
        stroke: alive ? sc_source.ring : "#3f3f46",
        strokeWidth: 1.5,
        strokeDasharray: "6 4",
        opacity: alive ? 0.45 : 0.2,
      },
      animated: alive,
    });
  }

  return { nodes, edges };
}

// ---------------------------------------------------------------------------
// Keyframe animation for flowing dashes (injected once)
// ---------------------------------------------------------------------------

const heroStyleId = "topology-hero-styles";

function ensureStyles() {
  if (typeof document === "undefined") return;
  if (document.getElementById(heroStyleId)) return;

  const style = document.createElement("style");
  style.id = heroStyleId;
  style.textContent = `
    .topology-hero-node:hover svg {
      transform: scale(1.12);
      transition: transform var(--dur-normal) var(--ease-out);
    }
    .topology-hero-node svg {
      transition: transform var(--dur-normal) var(--ease-out);
    }
    @media (prefers-reduced-motion: reduce) {
      .topology-hero-node:hover svg {
        transform: none;
      }
    }
  `;
  document.head.appendChild(style);
}

// ---------------------------------------------------------------------------
// TopologyHero component
// ---------------------------------------------------------------------------

const glassStyle: CSSProperties = {
  backdropFilter: "blur(16px)",
  WebkitBackdropFilter: "blur(16px)",
  boxShadow: "var(--shadow-panel)",
};

export function TopologyHero() {
  const status = useFastStatus();
  const router = useRouter();

  // Inject hover styles once
  useEffect(() => {
    ensureStyles();
    return () => {
      const el = document.getElementById(heroStyleId);
      if (el) el.remove();
    };
  }, []);

  const allNonHidden = useMemo(
    () => (status?.assets ?? []).filter((a) => !isHiddenAsset(a)),
    [status?.assets],
  );
  const deferredAllNonHidden = useDeferredValue(allNonHidden);

  const infraAssets = useMemo(
    () => deferredAllNonHidden.filter((a) => isInfraHost(a)),
    [deferredAllNonHidden],
  );

  const telemetry = useMemo(
    () => status?.telemetryOverview ?? [],
    [status?.telemetryOverview],
  );
  const deferredTelemetry = useDeferredValue(telemetry);
  const heroAssetIDs = useMemo(
    () => [...infraAssets]
      .sort((left, right) => left.name.localeCompare(right.name))
      .slice(0, HERO_MAX_NODES)
      .map((asset) => asset.id),
    [infraAssets],
  );
  const { dependencies } = useAssetDependencies(heroAssetIDs);

  const { nodes, edges } = useMemo(
    () => buildHeroGraph(infraAssets, deferredTelemetry, dependencies),
    [infraAssets, deferredTelemetry, dependencies],
  );

  const onNodeClick = useCallback(
    (_event: React.MouseEvent, node: Node) => {
      const assetId = (node.data as HeroNodeData | null)?.assetId;
      if (assetId) {
        router.push(`/nodes/${encodeURIComponent(assetId)}`);
      }
    },
    [router],
  );

  const isEmpty = nodes.length === 0;

  return (
    <div
      className="rounded-[calc(var(--radius-lg)+1px)] p-px"
      style={{
        background: "linear-gradient(135deg, rgba(var(--accent-rgb),0.2), rgba(var(--accent-rgb),0.03) 30%, transparent 50%, rgba(var(--accent-rgb),0.03) 70%, rgba(var(--accent-rgb),0.15))",
        backgroundSize: "200% 200%",
        animation: "border-travel 8s ease infinite",
      }}
    >
    <div
      className="relative bg-[var(--panel-glass)] rounded-lg overflow-hidden"
      style={glassStyle}
    >
      {/* Top-edge accent specular */}
      <div
        className="absolute top-0 left-[10%] right-[10%] h-px pointer-events-none z-10"
        style={{ background: "linear-gradient(90deg, transparent, rgba(var(--accent-rgb),0.3), transparent)" }}
      />

      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2.5 border-b border-[var(--panel-border)]">
        <span className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">// Topology</span>
        {!isEmpty && (
          <span className="text-[10px] text-[var(--muted)]">
            {infraAssets.length} asset{infraAssets.length !== 1 ? "s" : ""}
          </span>
        )}
      </div>

      {/* Graph */}
      <div
        className="w-full"
        style={{
          height: 280,
          background: "radial-gradient(ellipse at 50% 40%, rgba(var(--accent-rgb),0.04), transparent 70%)",
        }}
      >
        {isEmpty ? (
          <div className="flex h-full w-full items-center justify-center">
            <p className="text-xs text-[var(--muted)]">No infrastructure assets discovered yet</p>
          </div>
        ) : (
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            fitView
            fitViewOptions={{ padding: 0.25, maxZoom: 1.5 }}
            minZoom={0.5}
            maxZoom={2}
            onNodeClick={onNodeClick}
            nodesDraggable={false}
            nodesConnectable={false}
            elementsSelectable={false}
            panOnDrag
            zoomOnScroll={false}
            zoomOnPinch={false}
            zoomOnDoubleClick={false}
            preventScrolling={false}
            proOptions={{ hideAttribution: true }}
          />
        )}
      </div>
    </div>
    </div>
  );
}
