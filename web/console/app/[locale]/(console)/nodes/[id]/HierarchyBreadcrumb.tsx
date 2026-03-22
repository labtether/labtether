"use client";

import { useMemo } from "react";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { Skeleton } from "../../../../components/ui/Skeleton";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { useEdgeAncestors } from "../../../../hooks/useEdges";

type HierarchyBreadcrumbProps = {
  assetID: string;
};

export function HierarchyBreadcrumb({ assetID }: HierarchyBreadcrumbProps) {
  const status = useFastStatus();
  const { ancestors, loading } = useEdgeAncestors(assetID);

  // Sort ancestors by depth descending (root first)
  const sortedAncestors = useMemo(
    () => [...ancestors].sort((a, b) => b.depth - a.depth),
    [ancestors],
  );

  const assetsByID = useMemo(() => {
    const map = new Map<string, { id: string; name: string }>();
    for (const a of status?.assets ?? []) {
      map.set(a.id, a);
    }
    return map;
  }, [status?.assets]);

  if (!loading && ancestors.length === 0) {
    return null;
  }

  return (
    <nav aria-label="Asset hierarchy" className="flex items-center gap-1 text-xs text-[var(--muted)] mb-2 flex-wrap">
      {loading ? (
        <>
          <Skeleton width="60px" height="12px" />
          <ChevronRight size={11} className="shrink-0 opacity-40" />
          <Skeleton width="80px" height="12px" />
        </>
      ) : (
        sortedAncestors.map((ancestor, idx) => {
          const asset = assetsByID.get(ancestor.asset_id);
          const label = asset?.name ?? ancestor.asset_id;
          const isLast = idx === sortedAncestors.length - 1;
          return (
            <span key={ancestor.asset_id} className="flex items-center gap-1">
              {idx > 0 && <ChevronRight size={11} className="shrink-0 opacity-40" />}
              {isLast ? (
                <span className="text-[var(--text)]">{label}</span>
              ) : (
                <Link
                  href={`/nodes/${encodeURIComponent(ancestor.asset_id)}`}
                  className="hover:text-[var(--text)] transition-colors"
                  style={{ transitionDuration: "var(--dur-fast)" }}
                >
                  {label}
                </Link>
              )}
            </span>
          );
        })
      )}
    </nav>
  );
}
