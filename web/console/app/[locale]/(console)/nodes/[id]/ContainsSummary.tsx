"use client";

import { useMemo } from "react";
import Link from "next/link";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { useEdgeTree } from "../../../../hooks/useEdges";
import { assetCategory, CATEGORIES } from "../../../../console/taxonomy";

type ContainsSummaryProps = {
  assetID: string;
};

export function ContainsSummary({ assetID }: ContainsSummaryProps) {
  const { nodes, loading } = useEdgeTree(assetID, 1);
  const status = useFastStatus();

  const assetsByID = useMemo(() => {
    const map = new Map<string, { id: string; name: string; type: string }>();
    for (const a of status?.assets ?? []) {
      map.set(a.id, a);
    }
    return map;
  }, [status?.assets]);

  const categoryCounts = useMemo(() => {
    // nodes includes the root itself at depth 0; only count depth 1 (direct children)
    const children = nodes.filter((n) => n.depth === 1);
    const counts = new Map<string, number>();
    for (const child of children) {
      const asset = assetsByID.get(child.asset_id);
      if (!asset) continue;
      const cat = assetCategory(asset.type);
      if (!cat) continue;
      counts.set(cat, (counts.get(cat) ?? 0) + 1);
    }
    return counts;
  }, [nodes, assetsByID]);

  const totalChildren = useMemo(
    () => nodes.filter((n) => n.depth === 1).length,
    [nodes],
  );

  if (loading || totalChildren === 0) {
    return null;
  }

  // Build summary parts in CATEGORIES display order
  const parts: string[] = [];
  for (const cat of CATEGORIES) {
    const count = categoryCounts.get(cat.slug);
    if (count && count > 0) {
      parts.push(`${cat.label}: ${count}`);
    }
  }

  // Count children with unknown category
  const knownCount = Array.from(categoryCounts.values()).reduce((sum, v) => sum + v, 0);
  const unknownCount = totalChildren - knownCount;
  if (unknownCount > 0) {
    parts.push(`Other: ${unknownCount}`);
  }

  if (parts.length === 0) {
    return null;
  }

  return (
    <div className="flex items-center gap-3 mt-2 mb-3 flex-wrap">
      <span className="text-xs text-[var(--muted)]">
        Contains: {parts.join(" \u00b7 ")}
      </span>
      <Link
        href={`/topology?focus=${encodeURIComponent(assetID)}`}
        className="text-xs text-[var(--accent)] hover:underline transition-colors"
        style={{ transitionDuration: "var(--dur-fast)" }}
      >
        View full tree in Topology &rarr;
      </Link>
    </div>
  );
}
