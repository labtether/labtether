"use client";

import { useTranslations } from "next-intl";
import { useMemo } from "react";
import { Link } from "../../../../../i18n/navigation";
import { useStatusAssetNameMap } from "../../../../contexts/StatusContext";
import { useWebServices } from "../../../../hooks/useWebServices";
import { ServicesDiscoveryOverviewCard } from "../ServicesDiscoveryOverviewCard";
import { buildDiscoveryOverview } from "../servicesDiscoveryHelpers";

/** The agents' latest discovery cycles, and where the discovery defaults for new agents are set. */
export default function DiscoveryTab() {
  const t = useTranslations("services.discovery");
  const assetNameMap = useStatusAssetNameMap();
  const { discoveryStats, loading } = useWebServices({ detailLevel: "compact" });
  const overview = useMemo(() => buildDiscoveryOverview(discoveryStats, assetNameMap), [assetNameMap, discoveryStats]);

  return (
    <>
      <div className="sv-sh">
        <h2>{t("title")}</h2>
        <Link href="/settings#discovery" className="sv-sh-link">{t("defaults")}</Link>
      </div>
      {overview.hostCount > 0 ? (
        <ServicesDiscoveryOverviewCard discoveryOverview={overview} />
      ) : (
        <div className="sv-sheet sv-empty">
          <b>{loading ? t("loading") : t("noneTitle")}</b>
          {!loading && <span>{t("noneBody")}</span>}
        </div>
      )}
    </>
  );
}
