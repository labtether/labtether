"use client";

import { useMemo } from "react";
import type { Group } from "../console/models";

type GroupParentSelectProps = {
  groups: Group[];
  value: string; // current parent_group_id or ""
  onChange: (id: string) => void;
  excludeGroupId?: string; // exclude self + descendants
  disabled?: boolean;
  label?: string;
};

type FlatOption = {
  id: string;
  label: string;
};

/** Collect the full set of descendant IDs for a given root ID. */
function collectDescendantIds(rootId: string, groups: Group[]): Set<string> {
  const result = new Set<string>();
  const queue = [rootId];
  while (queue.length > 0) {
    const current = queue.pop()!;
    for (const g of groups) {
      if (g.parent_group_id === current) {
        result.add(g.id);
        queue.push(g.id);
      }
    }
  }
  return result;
}

/** Build a flat, depth-annotated option list from a flat Group array. */
function buildFlatOptions(
  groups: Group[],
  excludeGroupId: string | undefined
): FlatOption[] {
  // Determine which IDs to exclude (self + all descendants).
  const excluded = new Set<string>();
  if (excludeGroupId) {
    excluded.add(excludeGroupId);
    const descendants = collectDescendantIds(excludeGroupId, groups);
    descendants.forEach((id) => excluded.add(id));
  }

  const visible = groups.filter((g) => !excluded.has(g.id));
  const visibleIds = new Set(visible.map((g) => g.id));

  // Sort helper: sort_order ascending, then name ascending.
  const sorted = [...visible].sort((a, b) => {
    if (a.sort_order !== b.sort_order) return a.sort_order - b.sort_order;
    return a.name.localeCompare(b.name);
  });

  const options: FlatOption[] = [];

  function walk(parentId: string | undefined, depth: number) {
    const children = sorted.filter((g) => {
      if (parentId === undefined) {
        // Root level: include groups with no parent or orphaned parent
        return !g.parent_group_id || !visibleIds.has(g.parent_group_id);
      }
      return g.parent_group_id === parentId;
    });
    for (const g of children) {
      const indent = "\u00a0\u00a0\u00a0\u00a0".repeat(depth); // 4 NBSP per level
      const prefix = depth > 0 ? "\u251c\u2500\u2500 " : ""; // ├── for non-root
      options.push({ id: g.id, label: `${indent}${prefix}${g.name}` });
      walk(g.id, depth + 1);
    }
  }

  walk(undefined, 0);

  return options;
}

export function GroupParentSelect({
  groups,
  value,
  onChange,
  excludeGroupId,
  disabled = false,
  label,
}: GroupParentSelectProps) {
  const options = useMemo(
    () => buildFlatOptions(groups, excludeGroupId),
    [groups, excludeGroupId]
  );

  return (
    <div className="flex flex-col gap-1">
      {label && (
        <span className="text-[10px] text-[var(--muted)]">{label}</span>
      )}
      <div className="relative">
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          className="w-full appearance-none rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 pr-8 text-sm text-[var(--text)] outline-none select-chevron focus:border-[var(--accent)] focus:shadow-[0_0_0_3px_var(--accent-subtle)] disabled:text-[var(--text-disabled)] disabled:cursor-not-allowed transition-[border-color,box-shadow]"
          style={{ transitionDuration: "var(--dur-fast)" }}
        >
          <option value="">(None — top level)</option>
          {options.map((opt) => (
            <option key={opt.id} value={opt.id}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}
