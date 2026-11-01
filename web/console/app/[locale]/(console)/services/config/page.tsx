"use client";

import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import { PageHeader } from "../../../../components/PageHeader";
import ManualServicesTab from "./ManualServicesTab";
import OverridesTab from "./OverridesTab";
import GroupingMergeTab from "./GroupingMergeTab";
import IconsTab from "./IconsTab";
import DiscoveryTab from "./DiscoveryTab";

type ConfigTab = "manual" | "overrides" | "grouping" | "icons" | "discovery";

const TAB_ORDER: ConfigTab[] = ["manual", "overrides", "grouping", "icons", "discovery"];

/** Service settings: services added by hand, changes to found services, URL grouping and merging, icons, discovery. */
export default function ServiceConfigPage() {
  const t = useTranslations("services");
  const searchParams = useSearchParams();
  const rawTab = searchParams.get("tab") as ConfigTab | null;
  const activeTab: ConfigTab = rawTab && TAB_ORDER.includes(rawTab) ? rawTab : "manual";
  const assetParam = searchParams.get("asset") ?? "";

  function tabHref(tab: ConfigTab): string {
    const params = new URLSearchParams({ tab });
    if (assetParam) params.set("asset", assetParam);
    return `/services/config?${params.toString()}`;
  }

  return (
    <div className="sv-cfg">
      <nav className="sv-crumb" aria-label={t("config.crumb")}>
        <Link href="/services">{t("title")}</Link>
      </nav>
      <PageHeader title={t("config.title")} />
      <nav className="lt-tabs" aria-label={t("config.title")}>
        {TAB_ORDER.map((tab) => (
          <Link key={tab} href={tabHref(tab)} aria-current={tab === activeTab ? "page" : undefined}>
            {t(`config.tabs.${tab}`)}
          </Link>
        ))}
      </nav>
      <section aria-label={t(`config.tabs.${activeTab}`)}>
        {activeTab === "manual" && <ManualServicesTab prefilledAssetId={assetParam || undefined} />}
        {activeTab === "overrides" && <OverridesTab />}
        {activeTab === "grouping" && <GroupingMergeTab />}
        {activeTab === "icons" && <IconsTab />}
        {activeTab === "discovery" && <DiscoveryTab />}
      </section>
    </div>
  );
}
