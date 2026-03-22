"use client";

import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { ChevronLeft } from "lucide-react";
import { Link } from "../../../../../i18n/navigation";
import ManualServicesTab from "./ManualServicesTab";
import OverridesTab from "./OverridesTab";
import GroupingMergeTab from "./GroupingMergeTab";
import IconsTab from "./IconsTab";

type ConfigTab = "manual" | "overrides" | "grouping" | "icons";

const TAB_ORDER: ConfigTab[] = ["manual", "overrides", "grouping", "icons"];

export default function ServiceConfigPage() {
  const t = useTranslations("services");
  const searchParams = useSearchParams();

  const rawTab = searchParams.get("tab");
  const activeTab: ConfigTab =
    rawTab === "overrides" || rawTab === "grouping" || rawTab === "icons"
      ? rawTab
      : "manual";

  const assetParam = searchParams.get("asset") ?? "";
  const prefilledAsset = assetParam || undefined;

  function buildTabHref(tab: ConfigTab): string {
    const params = new URLSearchParams();
    params.set("tab", tab);
    if (assetParam) {
      params.set("asset", assetParam);
    }
    return `/services/config?${params.toString()}`;
  }

  return (
    <div className="flex flex-col flex-1 overflow-hidden">
      {/* Page header */}
      <div className="flex flex-col gap-1 border-b border-[var(--line)] px-4 pb-3 pt-4 md:px-6">
        <Link
          href="/services"
          className="inline-flex items-center gap-1 text-xs text-[var(--muted)] hover:text-[var(--text)] transition-colors w-fit"
        >
          <ChevronLeft className="h-3 w-3" />
          {t("config.backToServices")}
        </Link>
        <h1 className="text-lg font-semibold text-[var(--text)]">
          {t("config.title")}
        </h1>
      </div>

      {/* Tab bar */}
      <div
        role="tablist"
        aria-label={t("config.title")}
        className="flex gap-0 border-b border-[var(--line)] px-4 md:px-6"
      >
        {TAB_ORDER.map((tab) => {
          const isActive = tab === activeTab;
          return (
            <Link
              key={tab}
              href={buildTabHref(tab)}
              role="tab"
              aria-selected={isActive}
              aria-controls={`tabpanel-${tab}`}
              id={`tab-${tab}`}
              className={`relative px-3 py-2.5 text-[11px] font-semibold transition-colors ${
                isActive
                  ? "text-[var(--text)] after:absolute after:bottom-0 after:left-0 after:right-0 after:h-[2px] after:bg-[var(--accent)] after:rounded-t"
                  : "text-[var(--muted)] hover:text-[var(--text)]"
              }`}
            >
              {t(`config.tabs.${tab}`)}
            </Link>
          );
        })}
      </div>

      {/* Tab panels */}
      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        {TAB_ORDER.map((tab) => (
          <div
            key={tab}
            role="tabpanel"
            id={`tabpanel-${tab}`}
            aria-labelledby={`tab-${tab}`}
            hidden={tab !== activeTab}
          >
            {tab === "manual" && activeTab === "manual" && (
              <ManualServicesTab prefilledAssetId={prefilledAsset} />
            )}
            {tab === "overrides" && activeTab === "overrides" && (
              <OverridesTab />
            )}
            {tab === "grouping" && activeTab === "grouping" && (
              <GroupingMergeTab />
            )}
            {tab === "icons" && activeTab === "icons" && <IconsTab />}
          </div>
        ))}
      </div>
    </div>
  );
}
