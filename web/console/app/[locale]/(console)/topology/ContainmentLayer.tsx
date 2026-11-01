"use client";

import { memo, useState, useCallback } from "react";
import { ChevronDown } from "lucide-react";
import { useTranslations } from "next-intl";
import { friendlySourceLabel, friendlyTypeLabel } from "../../../console/taxonomy";
import { STATUS_TONE } from "./topologyCanvasTypes";

export interface ContainmentChild {
  id: string;
  name: string;
  type: string;
  source: string;
  status: string;
  port?: string;
}

interface ContainmentLayerProps {
  label: string;
  source: string;
  children: ContainmentChild[];
  maxVisible?: number;
}

/** Edit layout: what runs inside a card (its VMs, containers, datasets), one kind per layer, ten shown at first. */
function ContainmentLayerComponent({
  label,
  source,
  children,
  maxVisible = 10,
}: ContainmentLayerProps) {
  const t = useTranslations("topology");
  const [expanded, setExpanded] = useState(true);
  const [showAll, setShowAll] = useState(false);

  const toggle = useCallback(() => setExpanded((p) => !p), []);
  const toggleShowAll = useCallback(() => setShowAll((p) => !p), []);

  const visibleChildren = showAll ? children : children.slice(0, maxVisible);
  const hiddenCount = children.length - maxVisible;

  return (
    <div className="mp-layer">
      <button type="button" onClick={toggle} className="mp-layer-h" aria-expanded={expanded}>
        <ChevronDown aria-hidden="true" width={11} height={11} strokeWidth={2.2} style={{ rotate: expanded ? "0deg" : "-90deg" }} />
        <span className="mp-layer-nm">{label}</span>
        <span className="mp-layer-src">{friendlySourceLabel(source)}</span>
        <span className="mp-layer-n">{children.length}</span>
      </button>
      {expanded && (
        <div className="mp-layer-rows">
          {visibleChildren.map((child) => {
            const tone = STATUS_TONE[child.status] ?? "";
            return (
              <div key={child.id} className="mp-layer-row">
                <span className="mp-layer-dot">{tone && <i className={`mp-gd ${tone}`} aria-hidden="true" />}</span>
                <span className="mp-layer-cn">{child.name}</span>
                <span className="mp-layer-ct">{friendlyTypeLabel(child.type)}{child.port ? ` :${child.port}` : ""}</span>
              </div>
            );
          })}
          {hiddenCount > 0 && (
            <button type="button" onClick={toggleShowAll} className="mp-layer-more">
              {showAll ? t("fold.less") : t("fold.more", { count: hiddenCount })}
            </button>
          )}
        </div>
      )}
    </div>
  );
}

export const ContainmentLayer = memo(ContainmentLayerComponent);
