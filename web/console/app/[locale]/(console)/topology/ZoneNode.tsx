"use client";

import { memo, useState, useCallback, useEffect } from "react";
import { type NodeProps, NodeResizer } from "@xyflow/react";

// Zone color presets — subtle tints on glass surfaces
const ZONE_COLORS: Record<string, { border: string; bg: string; header: string; text: string; glow: string }> = {
  blue:   { border: "rgba(59,130,246,0.2)",  bg: "rgba(59,130,246,0.04)",  header: "rgba(59,130,246,0.08)", text: "#60a5fa", glow: "rgba(59,130,246,0.1)" },
  green:  { border: "rgba(34,197,94,0.2)",   bg: "rgba(34,197,94,0.04)",   header: "rgba(34,197,94,0.08)",  text: "#4ade80", glow: "rgba(34,197,94,0.1)" },
  purple: { border: "rgba(139,92,246,0.2)",  bg: "rgba(139,92,246,0.04)",  header: "rgba(139,92,246,0.08)", text: "#a78bfa", glow: "rgba(139,92,246,0.1)" },
  amber:  { border: "rgba(245,158,11,0.2)",  bg: "rgba(245,158,11,0.04)",  header: "rgba(245,158,11,0.08)", text: "#fbbf24", glow: "rgba(245,158,11,0.1)" },
  rose:   { border: "rgba(244,63,94,0.2)",   bg: "rgba(244,63,94,0.04)",   header: "rgba(244,63,94,0.08)",  text: "#fb7185", glow: "rgba(244,63,94,0.1)" },
  cyan:   { border: "rgba(6,182,212,0.2)",   bg: "rgba(6,182,212,0.04)",   header: "rgba(6,182,212,0.08)",  text: "#22d3ee", glow: "rgba(6,182,212,0.1)" },
};

/** When color is not a named key (e.g. a hex value), pick deterministically from the palette. */
function resolveZoneColor(color: string, zoneId: string): typeof ZONE_COLORS[keyof typeof ZONE_COLORS] {
  if (ZONE_COLORS[color]) return ZONE_COLORS[color];
  const names = Object.keys(ZONE_COLORS);
  let hash = 0;
  for (let i = 0; i < zoneId.length; i++) hash = ((hash << 5) - hash + zoneId.charCodeAt(i)) | 0;
  return ZONE_COLORS[names[Math.abs(hash) % names.length]];
}

export interface ZoneNodeData {
  zoneId: string;
  label: string;
  color: string;
  collapsed: boolean;
  assetCount: number;
  subZoneCount: number;
  /** When set to true externally, the node enters inline edit mode immediately. */
  renaming?: boolean;
  onLabelChange?: (zoneId: string, label: string) => void;
  onToggleCollapse?: (zoneId: string) => void;
  onDelete?: (zoneId: string) => void;
  onRenameDone?: (zoneId: string) => void;
  [key: string]: unknown;
}

function ZoneNodeComponent({ data, selected }: NodeProps) {
  const d = data as ZoneNodeData;
  const colors = resolveZoneColor(d.color, d.zoneId);
  const [editing, setEditing] = useState(false);
  const [editLabel, setEditLabel] = useState(d.label);

  // Enter edit mode when the parent sets renaming=true (e.g. from context menu)
  useEffect(() => {
    if (d.renaming && !editing) {
      setEditing(true);
      setEditLabel(d.label);
    }
  }, [d.renaming, d.label, editing]);

  const handleLabelDoubleClick = useCallback(() => {
    setEditing(true);
    setEditLabel(d.label);
  }, [d.label]);

  const commitLabel = useCallback(() => {
    setEditing(false);
    if (editLabel.trim() && editLabel !== d.label) {
      d.onLabelChange?.(d.zoneId, editLabel.trim());
    }
    d.onRenameDone?.(d.zoneId);
  }, [editLabel, d]);

  const cancelLabel = useCallback(() => {
    setEditing(false);
    d.onRenameDone?.(d.zoneId);
  }, [d]);

  return (
    <>
      <NodeResizer
        minWidth={200}
        minHeight={120}
        lineStyle={{ borderColor: selected ? colors.text : colors.border, borderWidth: 1.5 }}
        handleStyle={{ width: 8, height: 8, borderRadius: 4, background: colors.text, border: "none" }}
        isVisible={selected}
      />
      <div
        className="h-full w-full overflow-visible rounded-[var(--radius-lg)]"
        style={{
          border: `1px solid ${colors.border}`,
          background: `color-mix(in srgb, var(--panel-glass), ${colors.bg})`,
          backdropFilter: "blur(var(--blur-sm))",
          WebkitBackdropFilter: "blur(var(--blur-sm))",
          boxShadow: "var(--shadow-panel)",
        }}
      >
        {/* Header */}
        <div
          className="flex cursor-grab items-center gap-1.5 rounded-t-[calc(var(--radius-lg)-1px)] px-2.5 py-1.5 text-xs font-medium"
          style={{
            background: colors.header,
            color: colors.text,
            backdropFilter: "blur(8px)",
            WebkitBackdropFilter: "blur(8px)",
          }}
        >
          <button
            onClick={() => d.onToggleCollapse?.(d.zoneId)}
            className="text-[9px] opacity-50 hover:opacity-100 transition-opacity"
          >
            {d.collapsed ? "\u25B6" : "\u25BC"}
          </button>

          {editing ? (
            <input
              autoFocus
              value={editLabel}
              onChange={e => setEditLabel(e.target.value)}
              onBlur={commitLabel}
              onKeyDown={e => { if (e.key === "Enter") commitLabel(); if (e.key === "Escape") cancelLabel(); }}
              className="flex-1 bg-transparent border-none outline-none text-xs font-semibold"
              style={{ color: colors.text }}
            />
          ) : (
            <span className="flex-1 truncate" onDoubleClick={handleLabelDoubleClick}>
              {d.label}
            </span>
          )}

          <span className="text-[10px] opacity-40 font-normal">
            {d.assetCount > 0 ? d.assetCount : ""}
          </span>
        </div>

        {/* Body -- collapsed shows summary pill */}
        {d.collapsed ? (
          <div className="px-2.5 py-2">
            <span className="inline-flex items-center gap-1 rounded bg-[var(--surface)] px-2 py-0.5 text-[10px] text-[var(--muted)]">
              {d.assetCount} asset{d.assetCount !== 1 ? "s" : ""}
              {d.subZoneCount > 0 && `, ${d.subZoneCount} sub-zone${d.subZoneCount !== 1 ? "s" : ""}`}
            </span>
          </div>
        ) : (
          <div className="p-2">
            {/* Asset cards will be rendered as separate React Flow nodes inside this zone's bounds */}
          </div>
        )}
      </div>
    </>
  );
}

export const ZoneNode = memo(ZoneNodeComponent);
