"use client";

import { useEffect, useRef, useState } from "react";
import { SlidersHorizontal, X } from "lucide-react";

interface ServicesFilterPopoverProps {
  sourceFilter: string;
  onSourceFilterChange: (value: string) => void;
  statusFilter: string;
  onStatusFilterChange: (value: string) => void;
  healthFilter: string;
  onHealthFilterChange: (value: string) => void;
  showHidden: boolean;
  onShowHiddenChange: (value: boolean) => void;
}

function countActiveFilters(
  sourceFilter: string,
  statusFilter: string,
  healthFilter: string,
  showHidden: boolean,
): number {
  let count = 0;
  if (sourceFilter !== "all") count++;
  if (statusFilter !== "all") count++;
  if (healthFilter !== "all") count++;
  if (showHidden) count++;
  return count;
}

export function ServicesFilterPopover({
  sourceFilter,
  onSourceFilterChange,
  statusFilter,
  onStatusFilterChange,
  healthFilter,
  onHealthFilterChange,
  showHidden,
  onShowHiddenChange,
}: ServicesFilterPopoverProps) {
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!isOpen) return;

    const closeOnOutside = (event: MouseEvent | TouchEvent) => {
      const target = event.target;
      if (!(target instanceof Node)) {
        setIsOpen(false);
        return;
      }
      if (containerRef.current?.contains(target)) return;
      setIsOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setIsOpen(false);
    };

    window.addEventListener("mousedown", closeOnOutside, true);
    window.addEventListener("touchstart", closeOnOutside, true);
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      window.removeEventListener("mousedown", closeOnOutside, true);
      window.removeEventListener("touchstart", closeOnOutside, true);
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [isOpen]);

  const activeCount = countActiveFilters(sourceFilter, statusFilter, healthFilter, showHidden);

  function handleClearAll() {
    onSourceFilterChange("all");
    onStatusFilterChange("all");
    onHealthFilterChange("all");
    onShowHiddenChange(false);
  }

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setIsOpen((prev) => !prev)}
        className={`h-6 px-2.5 rounded-lg border text-xs font-medium transition-colors duration-[var(--dur-fast)] flex items-center gap-1.5 cursor-pointer ${
          isOpen
            ? "border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--text)]"
            : activeCount > 0
              ? "border-[var(--accent)] bg-[rgba(var(--accent-rgb),0.06)] text-[var(--text)]"
              : "border-[var(--line)] bg-[var(--surface)] text-[var(--muted)] hover:text-[var(--text)]"
        }`}
        style={{ backdropFilter: "blur(8px) saturate(1.4)" }}
      >
        <SlidersHorizontal className="w-3 h-3" />
        Filters
        {activeCount > 0 && (
          <span className="inline-flex items-center justify-center w-4 h-4 rounded-full bg-[var(--accent)] text-[var(--accent-contrast)] text-[10px] font-bold leading-none">
            {activeCount}
          </span>
        )}
      </button>

      {isOpen && (
        <div
          className="absolute right-0 top-full z-30 mt-2 w-56 rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)] p-3 shadow-xl flex flex-col gap-3"
          style={{ backdropFilter: "blur(16px) saturate(1.5)" }}
        >
          <div className="flex flex-col gap-1.5">
            <label className="text-[10px] font-medium text-[var(--muted)] uppercase tracking-wider">Source</label>
            <select
              aria-label="Source Filter"
              value={sourceFilter}
              onChange={(e) => onSourceFilterChange(e.target.value)}
              className="h-7 px-2 pr-6 rounded-md border border-[var(--line)] bg-[var(--surface)] text-xs text-[var(--text)] appearance-none cursor-pointer select-chevron w-full"
            >
              <option value="all">All Sources</option>
              <option value="docker">Docker</option>
              <option value="proxy">Proxy</option>
              <option value="scan">Scan</option>
              <option value="manual">Manual</option>
            </select>
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="text-[10px] font-medium text-[var(--muted)] uppercase tracking-wider">Status</label>
            <select
              aria-label="Status Filter"
              value={statusFilter}
              onChange={(e) => onStatusFilterChange(e.target.value)}
              className="h-7 px-2 pr-6 rounded-md border border-[var(--line)] bg-[var(--surface)] text-xs text-[var(--text)] appearance-none cursor-pointer select-chevron w-full"
            >
              <option value="all">All Status</option>
              <option value="up">Up</option>
              <option value="down">Down</option>
              <option value="unknown">Unknown</option>
            </select>
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="text-[10px] font-medium text-[var(--muted)] uppercase tracking-wider">Health</label>
            <select
              aria-label="Health Filter"
              value={healthFilter}
              onChange={(e) => onHealthFilterChange(e.target.value)}
              className="h-7 px-2 pr-6 rounded-md border border-[var(--line)] bg-[var(--surface)] text-xs text-[var(--text)] appearance-none cursor-pointer select-chevron w-full"
            >
              <option value="all">All Health</option>
              <option value="unstable">Unstable (&lt;95% uptime)</option>
              <option value="changed_recently">Changed Recently</option>
            </select>
          </div>

          <label className="flex items-center gap-1.5 text-xs text-[var(--text)] cursor-pointer select-none">
            <input
              type="checkbox"
              checked={showHidden}
              onChange={(e) => onShowHiddenChange(e.target.checked)}
              className="accent-[var(--accent)]"
            />
            Show Hidden Services
          </label>

          {activeCount > 0 && (
            <>
              <div className="border-t border-[var(--line)]" />
              <button
                type="button"
                onClick={handleClearAll}
                className="flex items-center justify-center gap-1.5 h-7 rounded-md text-xs font-medium text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer bg-transparent border-none w-full"
              >
                <X className="w-3 h-3" />
                Clear All Filters
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}
