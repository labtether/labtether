"use client";

import { HERO_MAX_NODES,HeroNodeData,STATUS_COLORS,buildHeroGraph } from './topologyHeroModel';


import {
Handle,
Position,
ReactFlow,
type Node,
type NodeProps
} from "@xyflow/react";
import { useCallback,useDeferredValue,useEffect,useMemo,type CSSProperties } from "react";
import { useRouter } from "../../i18n/navigation";
import { useAssetDependencies } from "../[locale]/(console)/topology/useAssetDependencies";
import { isHiddenAsset,isInfraHost } from "../console/taxonomy";
import { useFastStatus } from "../contexts/StatusContext";

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
