"use client";

import { ChevronRight } from "lucide-react";
import type { PanelDef, PanelContext } from "./devicePanels";

type DeviceCapabilityGridProps = {
  panels: PanelDef[];
  context: PanelContext;
  onOpenPanel: (panelId: string) => void;
};

export function DeviceCapabilityGrid({ panels, context, onOpenPanel }: DeviceCapabilityGridProps) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 stagger-children">
      {panels.map((panel) => {
        const summaryLines = panel.summary(context);
        return (
          <button
            type="button"
            key={panel.id}
            onClick={() => onOpenPanel(panel.id)}
            className="group flex flex-col rounded-lg border border-[var(--line)] bg-[var(--panel-glass)]
              px-4 py-3.5 text-left hover-lift press-effect
              transition-[border-color,box-shadow,transform] hover:border-[var(--accent)]/40
              hover:shadow-[0_0_12px_var(--accent-glow)] min-h-[72px]"
            style={{ transitionDuration: "var(--dur-fast)" }}
          >
            {/* Top: icon + label + chevron */}
            <div className="flex items-center gap-2.5 w-full">
              <span className="flex items-center justify-center h-7 w-7 rounded-md bg-[var(--accent-subtle)] shrink-0">
                <panel.icon
                  size={14}
                  className="text-[var(--accent-text)] group-hover:text-[var(--accent)] transition-colors"
                />
              </span>
              <span className="text-sm font-medium text-[var(--text)] flex-1 truncate">
                {panel.label}
              </span>
              <ChevronRight
                size={14}
                className="text-[var(--muted)] opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
              />
            </div>
            {/* Summary */}
            <p className="text-[11px] text-[var(--muted)] mt-1.5 truncate pl-[38px]">
              {summaryLines.join(" · ")}
            </p>
            {/* Connection badges */}
            {panel.id === "connect" && context.connectionBadges.length > 0 && (
              <div className="flex flex-wrap gap-1.5 mt-2 pl-[38px]">
                {context.connectionBadges.map((badge, i) => (
                  <span key={i} className="inline-flex items-center gap-1 text-[10px] text-[var(--muted)] bg-[var(--hover)] px-2 py-0.5 rounded">
                    <span className={`inline-block h-1.5 w-1.5 rounded-full ${
                      badge.status === "ok" ? "bg-[var(--ok)]" : badge.status === "bad" ? "bg-[var(--bad)]" : "bg-[var(--muted)]"
                    }`} />
                    {badge.label}
                  </span>
                ))}
              </div>
            )}
          </button>
        );
      })}
    </div>
  );
}
