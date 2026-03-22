"use client";

import { ServicesFilterPopover } from "./ServicesFilterPopover";

interface ServicesFiltersBarProps {
  totalCount: number;
  activeCategories: string[];
  categoryFilter: string;
  onCategoryFilterChange: (value: string) => void;
  hosts: Map<string, string>;
  hostFilter: string;
  onHostFilterChange: (value: string) => void;
  sourceFilter: string;
  onSourceFilterChange: (value: string) => void;
  healthFilter: string;
  onHealthFilterChange: (value: string) => void;
  sortMode: string;
  onSortModeChange: (value: string) => void;
  showHidden: boolean;
  onShowHiddenChange: (value: boolean) => void;
  statusFilter: string;
  onStatusFilterChange: (value: string) => void;
}

export function ServicesFiltersBar({
  totalCount,
  activeCategories,
  categoryFilter,
  onCategoryFilterChange,
  hosts,
  hostFilter,
  onHostFilterChange,
  sourceFilter,
  onSourceFilterChange,
  healthFilter,
  onHealthFilterChange,
  sortMode,
  onSortModeChange,
  showHidden,
  onShowHiddenChange,
  statusFilter,
  onStatusFilterChange,
}: ServicesFiltersBarProps) {
  if (totalCount <= 0) {
    return null;
  }

  return (
    <div className="flex flex-col gap-2">
      {/* Row 1: Category pills */}
      <div className="flex items-center gap-1.5 flex-wrap">
        <button
          type="button"
          onClick={() => onCategoryFilterChange("All")}
          className={`h-6 px-2.5 rounded-full text-xs font-medium transition-colors duration-[var(--dur-fast)] cursor-pointer ${
            categoryFilter === "All"
              ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
              : "bg-[var(--surface)] text-[var(--muted)] hover:text-[var(--text)]"
          }`}
        >
          All
        </button>
        {activeCategories.map((cat) => (
          <button
            key={cat}
            type="button"
            onClick={() => onCategoryFilterChange(categoryFilter === cat ? "All" : cat)}
            className={`h-6 px-2.5 rounded-full text-xs font-medium transition-colors duration-[var(--dur-fast)] cursor-pointer ${
              categoryFilter === cat
                ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
                : "bg-[var(--surface)] text-[var(--muted)] hover:text-[var(--text)]"
            }`}
          >
            {cat}
          </button>
        ))}
      </div>

      {/* Row 2: Host + Sort + Filter popover */}
      <div className="flex items-center gap-2">
        {hosts.size > 1 && (
          <select
            value={hostFilter}
            onChange={(event) => onHostFilterChange(event.target.value)}
            aria-label="Host Filter"
            className="h-6 px-2 pr-6 rounded-lg border border-[var(--line)] bg-[var(--surface)] text-xs text-[var(--text)] appearance-none cursor-pointer select-chevron"
            style={{ backdropFilter: "blur(8px) saturate(1.4)" }}
          >
            <option value="all">All Hosts</option>
            {Array.from(hosts.entries()).map(([id, name]) => (
              <option key={id} value={id}>
                {name}
              </option>
            ))}
          </select>
        )}

        <select
          value={sortMode}
          onChange={(event) => onSortModeChange(event.target.value)}
          aria-label="Sort Mode"
          className="h-6 px-2 pr-6 rounded-lg border border-[var(--line)] bg-[var(--surface)] text-xs text-[var(--text)] appearance-none cursor-pointer select-chevron"
          style={{ backdropFilter: "blur(8px) saturate(1.4)" }}
        >
          <option value="default">Sort: Default</option>
          <option value="most_unstable">Sort: Most Unstable</option>
          <option value="uptime_high">Sort: Uptime High-Low</option>
          <option value="recently_changed">Sort: Recently Changed</option>
        </select>

        <ServicesFilterPopover
          sourceFilter={sourceFilter}
          onSourceFilterChange={onSourceFilterChange}
          statusFilter={statusFilter}
          onStatusFilterChange={onStatusFilterChange}
          healthFilter={healthFilter}
          onHealthFilterChange={onHealthFilterChange}
          showHidden={showHidden}
          onShowHiddenChange={onShowHiddenChange}
        />
      </div>
    </div>
  );
}
