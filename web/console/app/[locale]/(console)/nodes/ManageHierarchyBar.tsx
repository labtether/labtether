"use client";

import { Check, FolderPlus, Pencil } from "lucide-react";

type ManageHierarchyBarProps = {
  isManaging: boolean;
  onToggle: () => void;
  onCreateGroup: () => void;
};

export function ManageHierarchyBar({ isManaging, onToggle, onCreateGroup }: ManageHierarchyBarProps) {
  return (
    <div className="flex items-center gap-2">
      <button
        type="button"
        onClick={onToggle}
        className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium transition-[color,background-color,border-color,box-shadow] cursor-pointer
          ${
            isManaging
              ? "bg-[var(--accent)] text-[var(--accent-contrast)] shadow-[0_0_12px_var(--accent-glow)]"
              : "bg-[var(--surface)] border border-[var(--line)] text-[var(--muted)] hover:text-[var(--text)] hover:border-[var(--text-secondary)]"
          }`}
        style={{ transitionDuration: "var(--dur-fast)" }}
      >
        {isManaging ? (
          <>
            <Check size={12} />
            Done
          </>
        ) : (
          <>
            <Pencil size={12} />
            Manage
          </>
        )}
      </button>

      {isManaging ? (
        <button
          type="button"
          onClick={onCreateGroup}
          className="inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium
            bg-[var(--surface)] border border-[var(--line)] text-[var(--muted)]
            hover:text-[var(--text)] hover:border-[var(--text-secondary)] transition-colors cursor-pointer"
          style={{ transitionDuration: "var(--dur-fast)" }}
        >
          <FolderPlus size={12} />
          New Group
        </button>
      ) : null}
    </div>
  );
}
