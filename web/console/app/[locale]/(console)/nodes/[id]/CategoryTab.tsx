"use client";

import { useMemo } from "react";
import { Link } from "../../../../../i18n/navigation";
import { Card } from "../../../../components/ui/Card";
import { Badge } from "../../../../components/ui/Badge";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { assetFreshnessLabel, formatAge } from "../../../../console/formatters";
import { buildNodeMetadataSections } from "../../../../console/models";
import type { Asset } from "../../../../console/models";
import {
  assetCategory,
  childParentKey,
  friendlyTypeLabel,
  hostParentKey,
  isHiddenAsset,
  sortCategoryAssets,
} from "../../../../console/taxonomy";
import type { CategoryDef } from "../../../../console/taxonomy";

function CategoryAssetRow({ asset }: { asset: Asset }) {
  const freshness = assetFreshnessLabel(asset.last_seen_at);
  const freshnessStatus = freshness === "online" ? "ok" : freshness === "unresponsive" ? "pending" : "bad";
  const metadataSections = buildNodeMetadataSections(asset.metadata);
  const metadataFieldCount = metadataSections.reduce((count, section) => count + section.rows.length, 0);

  return (
    <li className="py-2.5">
      <div className="flex items-center gap-3">
        <Link href={`/nodes/${asset.id}`} className="text-sm font-medium text-[var(--accent)] hover:underline">
          {asset.name}
        </Link>
        <span className="text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]">
          {friendlyTypeLabel(asset.type)}
        </span>
        <Badge status={freshnessStatus} size="sm" />
        <span className="text-xs text-[var(--muted)]">{formatAge(asset.last_seen_at)}</span>
      </div>
      {metadataFieldCount > 0 ? (
        <details className="mt-1.5 text-xs">
          <summary className="cursor-pointer text-[var(--muted)] hover:text-[var(--text)] transition-colors duration-150">
            {metadataFieldCount} {metadataFieldCount === 1 ? "field" : "fields"}
          </summary>
          <div className="space-y-6 mt-2 pl-2">
            {metadataSections.map((section) => (
              <div key={`${asset.id}-${section.title}`} className="space-y-2">
                <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">{section.title}</p>
                <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5">
                  {section.rows.map((row) => (
                    <div key={`${asset.id}-${section.title}-${row.key}`}>
                      <dt className="text-xs text-[var(--muted)]">{row.label}</dt>
                      <dd className="text-xs text-[var(--text)]">{row.value}</dd>
                    </div>
                  ))}
                </dl>
              </div>
            ))}
          </div>
        </details>
      ) : null}
    </li>
  );
}

export function CategoryTab({ category, hostAsset }: {
  category: CategoryDef;
  hostAsset: Asset;
}) {
  const status = useFastStatus();
  const allAssets = useMemo(() => status?.assets ?? [], [status?.assets]);
  const parentKey = useMemo(() => hostParentKey(hostAsset), [hostAsset]);

  const categoryAssets = useMemo(() => {
    const matched = allAssets.filter((a) => {
      if (a.id === hostAsset.id) return false;
      if (isHiddenAsset(a)) return false;
      if (a.source !== hostAsset.source) return false;
      if (childParentKey(a) !== parentKey) return false;
      return assetCategory(a.type) === category.slug;
    });
    return sortCategoryAssets(matched);
  }, [allAssets, category.slug, hostAsset.id, hostAsset.source, parentKey]);

  // Build type breakdown chips
  const typeCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const a of categoryAssets) {
      counts.set(a.type, (counts.get(a.type) ?? 0) + 1);
    }
    return Array.from(counts.entries()).sort(([a], [b]) => a.localeCompare(b));
  }, [categoryAssets]);

  return (
    <Card className="mb-4">
      {/* Summary header */}
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-[var(--text)]">{category.label}</h2>
        <span className="text-xs text-[var(--muted)]">
          {categoryAssets.length} {categoryAssets.length === 1 ? "resource" : "resources"}
        </span>
      </div>
      {typeCounts.length > 0 && (
        <div className="flex flex-wrap gap-1.5 mb-3">
          {typeCounts.map(([type, count]) => (
            <span
              key={type}
              className="text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]"
            >
              {friendlyTypeLabel(type)}: {count}
            </span>
          ))}
        </div>
      )}

      {/* Asset list */}
      {categoryAssets.length === 0 ? (
        <p className="text-sm text-[var(--muted)] py-8 text-center">
          No {category.label.toLowerCase()} resources found.
        </p>
      ) : (
        <ul className="divide-y divide-[var(--line)]">
          {categoryAssets.map((asset) => (
            <CategoryAssetRow key={asset.id} asset={asset} />
          ))}
        </ul>
      )}
    </Card>
  );
}
