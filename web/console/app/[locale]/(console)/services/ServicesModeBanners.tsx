"use client";

interface ServicesModeBannersProps {
  layoutMode: boolean;
  totalCount: number;
  selectionMode: boolean;
  selectedFilteredCount: number;
  filteredCount: number;
  onResetLayoutOrder: () => void;
  onSelectAllFiltered: () => void;
  onClearSelection: () => void;
}

export function ServicesModeBanners({
  layoutMode,
  totalCount,
  selectionMode,
  selectedFilteredCount,
  filteredCount,
  onResetLayoutOrder,
  onSelectAllFiltered,
  onClearSelection,
}: ServicesModeBannersProps) {
  return (
    <>
      {layoutMode && totalCount > 0 && (
        <div className="rounded-lg border border-[var(--accent)]/40 bg-[var(--accent)]/10 px-3 py-2 flex items-center justify-between gap-3">
          <p className="text-xs text-[var(--text)]">
            Arrange mode is active. Drag cards to reorder services inside each
            category.
          </p>
          <button
            type="button"
            onClick={onResetLayoutOrder}
            className="h-6 px-2 rounded border border-[var(--line)] text-[10px] font-semibold text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
          >
            Reset Layout
          </button>
        </div>
      )}

      {selectionMode && totalCount > 0 && (
        <div className="rounded-lg border border-[var(--accent)]/40 bg-[var(--accent)]/10 px-3 py-2 flex items-center justify-between gap-3">
          <p className="text-xs text-[var(--text)]">
            Selection mode is active. {selectedFilteredCount} of {filteredCount}{" "}
            filtered service{filteredCount === 1 ? "" : "s"} selected.
          </p>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onSelectAllFiltered}
              className="h-6 px-2 rounded border border-[var(--line)] text-[10px] font-semibold text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
            >
              Select All Filtered
            </button>
            <button
              type="button"
              onClick={onClearSelection}
              className="h-6 px-2 rounded border border-[var(--line)] text-[10px] font-semibold text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
            >
              Clear
            </button>
          </div>
        </div>
      )}
    </>
  );
}
