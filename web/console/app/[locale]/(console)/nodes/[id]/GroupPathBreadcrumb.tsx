"use client";

import { useMemo } from "react";
import { ChevronRight } from "lucide-react";
import { Link } from "../../../../../i18n/navigation";
import type { Group } from "../../../../console/models";

type GroupPathBreadcrumbProps = {
  groupId: string | undefined;
  groups: Group[];
};

/** Walks the parent_group_id chain and returns groups root-first. */
function buildGroupPath(groups: Group[], leafId: string): Group[] {
  const byId = new Map(groups.map((g) => [g.id, g]));
  const path: Group[] = [];
  let current = byId.get(leafId);
  let guard = 0;
  while (current && guard < 20) {
    path.unshift(current);
    current = current.parent_group_id ? byId.get(current.parent_group_id) : undefined;
    guard++;
  }
  return path;
}

export function GroupPathBreadcrumb({ groupId, groups }: GroupPathBreadcrumbProps) {
  const path = useMemo(() => {
    if (!groupId) return [];
    return buildGroupPath(groups, groupId);
  }, [groupId, groups]);

  if (path.length === 0) return null;

  return (
    <nav aria-label="Group path" className="flex items-center gap-1 text-xs text-[var(--muted)] mb-2 flex-wrap">
      {path.map((group, idx) => {
        const isLast = idx === path.length - 1;
        return (
          <span key={group.id} className="flex items-center gap-1">
            {idx > 0 && <ChevronRight size={11} className="shrink-0 opacity-40" />}
            {isLast ? (
              <span className="text-[var(--text)]">{group.name}</span>
            ) : (
              <Link
                href={`/groups`}
                className="hover:text-[var(--text)] transition-colors"
                style={{ transitionDuration: "var(--dur-fast)" }}
              >
                {group.name}
              </Link>
            )}
          </span>
        );
      })}
    </nav>
  );
}
