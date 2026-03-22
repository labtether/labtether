"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Group } from "../../../console/models";

// ── Tree node type ──

export type GroupTreeItem = {
  id: string;
  group: Group;
  children: GroupTreeItem[];
  depth: number;
  expanded: boolean;
  deviceCount: number; // direct + descendant devices
};

// ── Persistence ──

const STORAGE_KEY = "labtether-group-tree-expanded";

function loadExpanded(): Set<string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return new Set<string>();
    const parsed: unknown = JSON.parse(raw);
    if (Array.isArray(parsed))
      return new Set(parsed.filter((v): v is string => typeof v === "string"));
  } catch {
    /* ignore */
  }
  return new Set<string>();
}

function saveExpanded(set: Set<string>): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify([...set]));
  } catch {
    /* ignore */
  }
}

// ── Hook ──

export function useGroupTree(params: {
  groups: Group[];
  deviceCountByGroup: Map<string, number>;
}): {
  tree: GroupTreeItem[];
  toggleExpand: (id: string) => void;
  expandAll: () => void;
  collapseAll: () => void;
} {
  const { groups, deviceCountByGroup } = params;

  const [expandedSet, setExpandedSet] = useState<Set<string>>(() =>
    loadExpanded(),
  );
  const initializedRef = useRef(false);

  // Persist on change
  useEffect(() => {
    saveExpanded(expandedSet);
  }, [expandedSet]);

  const structuralTree = useMemo(() => {
    // Build parent → children map
    const groupByID = new Map<string, Group>();
    const childsByParent = new Map<string, Group[]>();
    const rootGroups: Group[] = [];

    for (const g of groups) {
      groupByID.set(g.id, g);
    }

    for (const g of groups) {
      if (g.parent_group_id && groupByID.has(g.parent_group_id)) {
        const siblings = childsByParent.get(g.parent_group_id) ?? [];
        siblings.push(g);
        childsByParent.set(g.parent_group_id, siblings);
      } else {
        rootGroups.push(g);
      }
    }

    // Sort by sort_order, then name
    const sortGroups = (arr: Group[]): Group[] =>
      arr.sort((a, b) => {
        if (a.sort_order !== b.sort_order) return a.sort_order - b.sort_order;
        return a.name.localeCompare(b.name);
      });

    sortGroups(rootGroups);
    for (const [, children] of childsByParent) {
      sortGroups(children);
    }

    // Recursively compute device counts (direct + descendant)
    function computeDeviceCount(groupID: string): number {
      const direct = deviceCountByGroup.get(groupID) ?? 0;
      const children = childsByParent.get(groupID) ?? [];
      const nested = children.reduce(
        (sum, child) => sum + computeDeviceCount(child.id),
        0,
      );
      return direct + nested;
    }

    // Build tree recursively
    function buildNode(group: Group, depth: number): GroupTreeItem {
      const id = `group:${group.id}`;
      const childGroups = childsByParent.get(group.id) ?? [];
      const children: GroupTreeItem[] = childGroups.map((cg) =>
        buildNode(cg, depth + 1),
      );
      return {
        id,
        group,
        children,
        depth,
        expanded: false, // expansion applied separately
        deviceCount: computeDeviceCount(group.id),
      };
    }

    return rootGroups.map((g) => buildNode(g, 0));
  }, [groups, deviceCountByGroup]);

  // Apply expansion state (cheap — runs only on toggle changes)
  const tree = useMemo(() => {
    function applyExpansion(items: GroupTreeItem[]): GroupTreeItem[] {
      return items.map((item) => ({
        ...item,
        expanded: expandedSet.has(item.id),
        children: applyExpansion(item.children),
      }));
    }
    return applyExpansion(structuralTree);
  }, [structuralTree, expandedSet]);

  // Auto-expand all on first load if no saved state
  useEffect(() => {
    if (initializedRef.current) return;
    initializedRef.current = true;

    if (expandedSet.size > 0) return;

    const allIDs = new Set<string>();
    function collectIDs(items: GroupTreeItem[]) {
      for (const item of items) {
        allIDs.add(item.id);
        collectIDs(item.children);
      }
    }
    collectIDs(structuralTree);
    if (allIDs.size > 0) {
      setExpandedSet(allIDs);
    }
  }, [structuralTree, expandedSet.size]);

  const toggleExpand = useCallback((id: string) => {
    setExpandedSet((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const expandAll = useCallback(() => {
    const allIDs = new Set<string>();
    function collect(items: GroupTreeItem[]) {
      for (const item of items) {
        allIDs.add(item.id);
        collect(item.children);
      }
    }
    collect(structuralTree);
    setExpandedSet(allIDs);
  }, [structuralTree]);

  const collapseAll = useCallback(() => {
    setExpandedSet(new Set());
  }, []);

  return { tree, toggleExpand, expandAll, collapseAll };
}
