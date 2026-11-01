"use client";

import { memo, useState, useCallback, useEffect } from "react";
import { ChevronDown } from "lucide-react";
import { useTranslations } from "next-intl";
import { type NodeProps, NodeResizer } from "@xyflow/react";

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

/** Edit layout: a zone as a quiet box. Drag it by its title, resize it when selected, double-click the name to rename. */
function ZoneNodeComponent({ data, selected }: NodeProps) {
  const d = data as ZoneNodeData;
  const t = useTranslations("topology");
  const [editing, setEditing] = useState(false);
  const [editLabel, setEditLabel] = useState(d.label);

  // Enter edit mode when the parent sets renaming=true (from the zone's menu).
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
      <NodeResizer minWidth={200} minHeight={120} isVisible={selected} lineClassName="mp-zone-line" handleClassName="mp-zone-handle" />
      <div className={`mp-zone${selected ? " sel" : ""}`}>
        <div className="mp-zone-h cursor-grab">
          <button
            type="button"
            onClick={() => d.onToggleCollapse?.(d.zoneId)}
            className="mp-zone-x"
            aria-expanded={!d.collapsed}
            aria-label={t("edit.collapse")}
          >
            <ChevronDown aria-hidden="true" width={12} height={12} strokeWidth={2.2} style={{ rotate: d.collapsed ? "-90deg" : "0deg" }} />
          </button>
          {editing ? (
            <input
              autoFocus
              value={editLabel}
              onChange={(e) => setEditLabel(e.target.value)}
              onBlur={commitLabel}
              onKeyDown={(e) => { if (e.key === "Enter") commitLabel(); if (e.key === "Escape") cancelLabel(); }}
              className="mp-zone-in"
              aria-label={t("zones.name", { zone: d.label })}
            />
          ) : (
            <span className="mp-zone-nm" onDoubleClick={handleLabelDoubleClick}>{d.label}</span>
          )}
          {d.assetCount > 0 && <span className="mp-zone-n">{d.assetCount}</span>}
        </div>
        {d.collapsed && (
          <p className="mp-zone-sum">
            {t("edit.zoneSummary", { devices: d.assetCount, zones: d.subZoneCount })}
          </p>
        )}
      </div>
    </>
  );
}

export const ZoneNode = memo(ZoneNodeComponent);
