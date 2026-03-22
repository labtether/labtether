"use client";

import { useState } from "react";
import { friendlySourceLabel } from "../../../../console/taxonomy";

type Facet = {
  asset_id: string;
  source: string;
  type: string;
};

type FacetTabsProps = {
  facets?: Facet[];
};

export function FacetTabs({ facets }: FacetTabsProps) {
  const [activeSource, setActiveSource] = useState<string | null>(null);

  if (!facets || facets.length === 0) {
    return null;
  }

  // Deduplicate sources
  const sources = Array.from(new Set(facets.map((f) => f.source)));

  const resolvedActive = activeSource ?? sources[0] ?? null;

  return (
    <div className="mt-2 mb-3">
      <div className="flex items-center gap-1 flex-wrap">
        <span className="text-xs text-[var(--muted)] mr-1">Sources:</span>
        {sources.map((source) => {
          const isActive = source === resolvedActive;
          return (
            <button
              key={source}
              type="button"
              onClick={() => setActiveSource(source)}
              className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium border transition-colors ${
                isActive
                  ? "bg-[var(--accent-subtle)] border-[var(--accent)]/40 text-[var(--accent-text)]"
                  : "border-[var(--line)] text-[var(--muted)] hover:text-[var(--text)] hover:border-[var(--line)]"
              }`}
              style={{ transitionDuration: "var(--dur-fast)" }}
            >
              {friendlySourceLabel(source)}
            </button>
          );
        })}
      </div>
    </div>
  );
}
