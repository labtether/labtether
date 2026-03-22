"use client";

import { memo, useState, useCallback } from "react";
import { Handle, Position, type NodeProps } from "@xyflow/react";
import { ContainmentLayer } from "./ContainmentLayer";
import type { ContainmentChild } from "./ContainmentLayer";
import { SOURCE_COLORS, STATUS_COLORS } from "./topologyCanvasTypes";

export interface ContainmentLayerData {
  label: string;
  source: string;
  children: ContainmentChild[];
}

export interface ContainmentCardData {
  assetId: string;
  name: string;
  type: string;
  sources: string[];
  status: string;
  summaryBadge: string;
  layers: ContainmentLayerData[];
  hasChildren: boolean;
  onSelect?: (assetId: string) => void;
  [key: string]: unknown;
}

function ContainmentCardComponent({ data }: NodeProps) {
  const d = data as ContainmentCardData;
  const [expanded, setExpanded] = useState(false);

  const statusColor = STATUS_COLORS[d.status] ?? STATUS_COLORS.unknown;
  const primarySource = d.sources[0] ?? "unknown";
  const primarySourceColor = SOURCE_COLORS[primarySource] ?? { bg: "rgba(113,113,122,0.12)", text: "#a1a1aa" };

  const toggleExpand = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    setExpanded((p) => !p);
  }, []);

  const handleHeaderClick = useCallback(() => {
    d.onSelect?.(d.assetId);
  }, [d]);

  return (
    <div
      className="group relative rounded-[var(--radius-md)] border border-[var(--panel-border)] bg-[var(--panel)] transition-all duration-[var(--dur-normal)] hover:border-[rgba(255,0,128,0.2)] hover:-translate-y-px hover:shadow-md hover:shadow-black/20"
      style={{ width: 280, boxShadow: "var(--shadow-panel)" }}
    >
      {/* Connection handles -- visible on hover */}
      <Handle
        type="target"
        id="left-target"
        position={Position.Left}
        className="!h-2 !w-2 !rounded-full !border-0 !bg-[var(--muted)] opacity-0 transition-opacity group-hover:opacity-60"
      />
      <Handle
        type="source"
        id="right-source"
        position={Position.Right}
        className="!h-2 !w-2 !rounded-full !border-0 !bg-[var(--muted)] opacity-0 transition-opacity group-hover:opacity-60"
      />

      {/* Header row */}
      <button
        onClick={handleHeaderClick}
        className="flex w-full items-center gap-1.5 rounded-t-[calc(var(--radius-md)-1px)] px-2.5 py-2 text-left transition-colors hover:bg-[var(--hover)]"
      >
        {/* Expand chevron (only if has children) */}
        {d.hasChildren ? (
          <span
            onClick={toggleExpand}
            className="cursor-pointer text-[9px] text-[var(--muted)] hover:text-[var(--text)]"
            style={{ transform: expanded ? "rotate(0deg)" : "rotate(-90deg)", transition: `transform var(--dur-fast) ease` }}
          >
            {"\u25BC"}
          </span>
        ) : (
          <span className="w-[9px]" />
        )}

        {/* Status dot */}
        <span
          className="inline-block h-2 w-2 shrink-0 rounded-full"
          style={{ background: statusColor, boxShadow: `0 0 6px ${statusColor}` }}
        />

        {/* Name */}
        <span className="flex-1 truncate text-xs font-medium text-[var(--text)]">
          {d.name}
        </span>

        {/* Type badge */}
        <span className="shrink-0 rounded px-1.5 py-px text-[8px] font-medium text-[var(--muted)] bg-[rgba(255,255,255,0.06)]">
          {d.type}
        </span>

        {/* Source badges */}
        {d.sources.map((src) => {
          const sc = SOURCE_COLORS[src] ?? { bg: "rgba(113,113,122,0.12)", text: "#a1a1aa" };
          return (
            <span
              key={src}
              className="shrink-0 rounded px-1 py-px text-[8px] font-medium"
              style={{ background: sc.bg, color: sc.text }}
            >
              {src}
            </span>
          );
        })}

        {/* Summary badge (collapsed) */}
        {!expanded && d.summaryBadge && (
          <span className="shrink-0 text-[9px] text-[var(--muted)]">
            {d.summaryBadge}
          </span>
        )}
      </button>

      {/* Expanded containment layers */}
      {expanded && d.layers.length > 0 && (
        <div className="border-t border-[var(--panel-border)] px-1.5 pb-2 pt-1">
          {d.layers.map((layer) => (
            <ContainmentLayer
              key={`${layer.source}-${layer.label}`}
              label={layer.label}
              source={layer.source}
              children={layer.children}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export const ContainmentCardNode = memo(ContainmentCardComponent);
