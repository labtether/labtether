"use client";

import { useMemo } from "react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { useEdgeTree } from "../../../../hooks/useEdges";
import { assetCategory, CATEGORIES } from "../../../../console/taxonomy";

type ContainsSummaryProps = {
  assetID: string;
};

/** A Details row: what this device directly contains, by kind ("Compute 3 · Storage 2"), with a link to Topology. */
export function ContainsSummary({ assetID }: ContainsSummaryProps) {
  const t = useTranslations("devices.page");
  const { nodes, loading } = useEdgeTree(assetID, 1);
  const status = useFastStatus();

  const parts = useMemo(() => {
    const typeByID = new Map((status?.assets ?? []).map((a) => [a.id, a.type]));
    // nodes includes the root itself at depth 0; only direct children count.
    const children = nodes.filter((n) => n.depth === 1);
    const counts = new Map<string, number>();
    let known = 0;
    for (const child of children) {
      const type = typeByID.get(child.asset_id);
      const cat = type ? assetCategory(type) : undefined;
      if (!cat) continue;
      counts.set(cat, (counts.get(cat) ?? 0) + 1);
      known += 1;
    }
    const out: Array<{ key: string; count: number }> = CATEGORIES.filter((c) => counts.get(c.slug)).map((c) => ({ key: c.slug, count: counts.get(c.slug) ?? 0 }));
    if (children.length > known) out.push({ key: "other", count: children.length - known });
    return out;
  }, [nodes, status?.assets]);

  if (loading || parts.length === 0) return null;

  return (
    <div>
      <dt>{t("details.contains")}</dt>
      <dd>
        <span>{parts.map((p) => `${t(`details.kinds.${p.key}`)} ${p.count}`).join(" · ")}</span>
        <Link className="dv-link" href={`/topology?focus=${encodeURIComponent(assetID)}`}>{t("details.topology")}</Link>
      </dd>
    </div>
  );
}
