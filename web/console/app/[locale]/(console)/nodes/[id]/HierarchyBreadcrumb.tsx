"use client";

import { useMemo } from "react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { useEdgeAncestors } from "../../../../hooks/useEdges";

type HierarchyBreadcrumbProps = {
  assetID: string;
  name: string;
};

/** The crumb above the device's name: Devices, then the devices it sits in (a VM's host), root first. */
export function HierarchyBreadcrumb({ assetID, name }: HierarchyBreadcrumbProps) {
  const t = useTranslations("devices.page");
  const status = useFastStatus();
  const { ancestors } = useEdgeAncestors(assetID);

  const trail = useMemo(() => {
    const names = new Map((status?.assets ?? []).map((a) => [a.id, a.name]));
    return [...ancestors]
      .sort((a, b) => b.depth - a.depth)
      .filter((a) => a.asset_id !== assetID)
      .map((a) => ({ id: a.asset_id, name: names.get(a.asset_id) ?? a.asset_id }));
  }, [ancestors, assetID, status?.assets]);

  return (
    <nav className="dv-crumb" aria-label={t("crumbLabel")}>
      <Link href="/nodes">{t("crumb")}</Link>
      {trail.map((a) => (
        <span key={a.id} className="dv-crumb-part">
          <span aria-hidden="true">/</span>
          <Link href={`/nodes/${encodeURIComponent(a.id)}`}>{a.name}</Link>
        </span>
      ))}
      <span aria-hidden="true">/</span>
      <span aria-current="page">{name}</span>
    </nav>
  );
}
