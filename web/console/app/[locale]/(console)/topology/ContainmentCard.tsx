"use client";

import { memo, useState, useCallback } from "react";
import { ChevronDown } from "lucide-react";
import { Handle, Position, type NodeProps } from "@xyflow/react";
import { friendlySourceLabel, friendlyTypeLabel } from "../../../console/taxonomy";
import { ContainmentLayer } from "./ContainmentLayer";
import type { ContainmentChild } from "./ContainmentLayer";
import { STATUS_TONE } from "./topologyCanvasTypes";

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

/** Edit layout: one device as a card inside its zone. Drag from its side dots to another card to connect the two. */
function ContainmentCardComponent({ data }: NodeProps) {
  const d = data as ContainmentCardData;
  const [expanded, setExpanded] = useState(false);
  const tone = STATUS_TONE[d.status] ?? "";

  const toggleExpand = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    setExpanded((p) => !p);
  }, []);

  const handleHeaderClick = useCallback(() => {
    d.onSelect?.(d.assetId);
  }, [d]);

  const meta = [friendlyTypeLabel(d.type), ...d.sources.map(friendlySourceLabel), !expanded ? d.summaryBadge : ""].filter(Boolean).join(" · ");

  return (
    <div className="mp-card">
      <Handle type="target" id="left-target" position={Position.Left} className="mp-handle" />
      <Handle type="source" id="right-source" position={Position.Right} className="mp-handle" />
      <div className="mp-card-h">
        {d.hasChildren ? (
          <button type="button" className="mp-card-x" onClick={toggleExpand} aria-expanded={expanded} aria-label={d.name}>
            <ChevronDown aria-hidden="true" width={12} height={12} strokeWidth={2.2} style={{ rotate: expanded ? "0deg" : "-90deg" }} />
          </button>
        ) : <span className="mp-card-x" />}
        <button type="button" className="mp-card-b" onClick={handleHeaderClick}>
          <span className="mp-card-nm">
            {tone && <i className={`mp-gd ${tone}`} aria-hidden="true" />}
            {d.name}
          </span>
          <span className="mp-card-m">{meta}</span>
        </button>
      </div>
      {expanded && d.layers.length > 0 && (
        <div className="mp-card-ls">
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
