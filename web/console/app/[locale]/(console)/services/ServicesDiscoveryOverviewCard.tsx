"use client";

import { useLocale, useTranslations } from "next-intl";
import { formatCheckTime } from "./servicesListModel";
import type { DiscoveryOverview } from "./servicesPageHelpers";

interface ServicesDiscoveryOverviewCardProps {
  discoveryOverview: DiscoveryOverview;
}

/**
 * What service discovery found in the agents' latest cycles: one row per source (on how many devices it runs, what it
 * finds per cycle, how long it takes) and the five devices that reported last. Nothing until an agent reports.
 */
export function ServicesDiscoveryOverviewCard({ discoveryOverview: o }: ServicesDiscoveryOverviewCardProps) {
  const t = useTranslations("services.discovery");
  const locale = useLocale();
  if (o.hostCount <= 0) return null;
  const last = formatCheckTime(o.latestCollectedAt, locale);

  return (
    <div className="sv-list sv-disc" role="region" aria-label={t("label")}>
      <p className="sv-disc-sum">
        {[t("devices", { count: o.hostCount }), t("perCycle", { count: o.averageDiscoveredServices }), t("cycleTime", { ms: o.averageCycleDurationMs }),
          last ? t("lastCycle", { time: last }) : ""].filter(Boolean).join(" · ")}
      </p>
      <div className="sv-row sv-cols sv-disc-row">
        <span>{t("source")}</span>
        <span>{t("enabled")}</span>
        <span className="sv-r">{t("found")}</span>
        <span className="sv-r">{t("time")}</span>
      </div>
      {o.sources.map((source) => (
        <div key={source.key} className="sv-row sv-disc-row" data-testid={`discovery-source-${source.key}`}>
          <span className="sv-nm">{t(`sources.${source.key}`)}</span>
          <span className="sv-grey">{t("enabledOn", { on: source.enabledHosts, total: o.hostCount })}</span>
          <span className="sv-ms">{source.servicesFound}</span>
          <span className="sv-ms">{source.hostsReported > 0 ? `${Math.round(source.totalDurationMs / source.hostsReported)} ms` : "—"}</span>
        </div>
      ))}
      <div className="sv-row sv-cols sv-disc-row">
        <span>{t("device")}</span>
        <span>{t("lastReport")}</span>
        <span className="sv-r">{t("services")}</span>
        <span className="sv-r">{t("time")}</span>
      </div>
      {o.hostRows.slice(0, 5).map((row) => (
        <div key={row.hostAssetID} className="sv-row sv-disc-row">
          <span className="sv-nm">{row.hostLabel}</span>
          <span className="sv-grey">{formatCheckTime(row.collectedAt, locale)}</span>
          <span className="sv-ms">{row.totalServices}</span>
          <span className="sv-ms">{row.cycleDurationMs} ms</span>
        </div>
      ))}
    </div>
  );
}
