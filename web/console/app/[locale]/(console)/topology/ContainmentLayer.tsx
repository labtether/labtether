"use client";

import { memo, useState, useCallback } from "react";
import { SOURCE_COLORS, STATUS_COLORS } from "./topologyCanvasTypes";

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

function ContainmentLayerComponent({
  label,
  source,
  children,
  maxVisible = 10,
}: ContainmentLayerProps) {
  const [expanded, setExpanded] = useState(true);
  const [showAll, setShowAll] = useState(false);

  const toggle = useCallback(() => setExpanded((p) => !p), []);
  const toggleShowAll = useCallback(() => setShowAll((p) => !p), []);

  const sourceColor = SOURCE_COLORS[source] ?? { bg: "rgba(113,113,122,0.12)", text: "#a1a1aa" };
  const visibleChildren = showAll ? children : children.slice(0, maxVisible);
  const hiddenCount = children.length - maxVisible;

  return (
    <div className="mt-1">
      {/* Layer header */}
      <button
        onClick={toggle}
        className="flex w-full items-center gap-1.5 rounded px-1.5 py-1 text-[10px] transition-colors duration-[var(--dur-fast)] hover:bg-[var(--hover)]"
      >
        <span className="text-[8px] text-[var(--muted)]" style={{ transform: expanded ? "rotate(0deg)" : "rotate(-90deg)", transition: `transform var(--dur-fast) ease` }}>
          {"\u25BC"}
        </span>
        <span className="font-medium text-[var(--text)]">{label}</span>
        <span
          className="rounded px-1.5 py-px text-[8px] font-medium"
          style={{ background: sourceColor.bg, color: sourceColor.text }}
        >
          {source}
        </span>
        <span className="ml-auto text-[9px] text-[var(--muted)]">{children.length}</span>
      </button>

      {/* Child rows */}
      {expanded && (
        <div
          className="ml-3 mt-0.5 space-y-px"
          style={{ borderLeft: `2px solid ${sourceColor.bg.replace("0.12", "0.3")}` }}
        >
          {visibleChildren.map((child) => {
            const dotColor = STATUS_COLORS[child.status] ?? STATUS_COLORS.unknown;
            return (
              <div
                key={child.id}
                className="flex items-center gap-1.5 rounded px-1.5 py-0.5 text-[10px] transition-colors duration-[var(--dur-fast)] hover:bg-[var(--hover)]"
              >
                <span
                  className="inline-block h-1.5 w-1.5 shrink-0 rounded-full"
                  style={{ background: dotColor, boxShadow: `0 0 6px ${dotColor}` }}
                />
                <span className="truncate text-[var(--text)]">{child.name}</span>
                <span className="shrink-0 text-[var(--muted)]">{child.type}</span>
                {child.port && (
                  <span className="shrink-0 font-[family-name:var(--font-jetbrains-mono)] text-[var(--muted)]">:{child.port}</span>
                )}
              </div>
            );
          })}
          {hiddenCount > 0 && !showAll && (
            <button
              onClick={toggleShowAll}
              className="px-1.5 py-0.5 text-[9px] text-[var(--accent)] hover:underline"
            >
              +{hiddenCount} more
            </button>
          )}
          {showAll && hiddenCount > 0 && (
            <button
              onClick={toggleShowAll}
              className="px-1.5 py-0.5 text-[9px] text-[var(--accent)] hover:underline"
            >
              show less
            </button>
          )}
        </div>
      )}
    </div>
  );
}

export const ContainmentLayer = memo(ContainmentLayerComponent);
