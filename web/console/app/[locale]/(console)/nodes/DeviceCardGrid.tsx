"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import { ChevronRight, Server } from "lucide-react";
import { EmptyState } from "../../../components/ui/EmptyState";
import { DeviceCard } from "./DeviceCard";
import type { DeviceGroupSection } from "./nodesPageUtils";

type DeviceCardGridProps = {
  sections: DeviceGroupSection[];
};

function GroupDivider({
  section,
  expanded,
  onToggle,
}: {
  section: DeviceGroupSection;
  expanded: boolean;
  onToggle: () => void;
}) {
  const allOnline = section.counts.issues === 0;
  const statusText = allOnline
    ? "all online"
    : [
        section.counts.offline > 0 ? `${section.counts.offline} offline` : "",
        section.counts.unresponsive > 0 ? `${section.counts.unresponsive} unresponsive` : "",
        section.counts.unknown > 0 ? `${section.counts.unknown} unknown` : "",
      ]
        .filter(Boolean)
        .join(", ");

  return (
    <button
      type="button"
      onClick={onToggle}
      className="group flex w-full items-center gap-3 py-2 text-left"
      aria-expanded={expanded}
    >
      <ChevronRight
        size={14}
        className={`shrink-0 text-[var(--muted)] transition-transform ${expanded ? "rotate-90" : ""}`}
        style={{ transitionDuration: "var(--dur-fast)" }}
      />
      <span className="text-xs font-semibold uppercase tracking-wider text-[var(--muted)]">
        {section.groupLabel}
      </span>
      <span className="h-px flex-1 bg-[var(--line)]" />
      <span className="text-[11px] text-[var(--muted)]">
        {section.devices.length} device{section.devices.length !== 1 ? "s" : ""} &middot; {statusText}
      </span>
    </button>
  );
}

export function DeviceCardGrid({ sections }: DeviceCardGridProps) {
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
  const initialized = useRef(false);

  // Auto-expand on first render: groups with issues, or all if none have issues
  useEffect(() => {
    if (initialized.current || sections.length === 0) return;
    const withIssues = sections.filter(s => s.counts.issues > 0).map(s => s.groupID);
    setExpandedGroups(new Set(withIssues.length > 0 ? withIssues : sections.map(s => s.groupID)));
    initialized.current = true;
  }, [sections]);

  const toggleGroup = useCallback((groupID: string) => {
    initialized.current = true;
    setExpandedGroups(prev => {
      const next = new Set(prev);
      if (next.has(groupID)) next.delete(groupID);
      else next.add(groupID);
      return next;
    });
  }, []);

  if (sections.length === 0) {
    return (
      <EmptyState
        icon={Server}
        title="No devices match your search"
        description="Try a different search term."
      />
    );
  }

  const showGroupHeaders = sections.length > 1 || (sections.length === 1 && sections[0].groupID !== "unassigned");

  return (
    <div className="space-y-4">
      {sections.map(section => {
        const isExpanded = expandedGroups.has(section.groupID);
        return (
          <div key={section.groupID}>
            {showGroupHeaders ? (
              <GroupDivider
                section={section}
                expanded={isExpanded}
                onToggle={() => toggleGroup(section.groupID)}
              />
            ) : null}
            {(!showGroupHeaders || isExpanded) ? (
              <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
                {section.devices.map(card => (
                  <DeviceCard key={card.asset.id} card={card} />
                ))}
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}
