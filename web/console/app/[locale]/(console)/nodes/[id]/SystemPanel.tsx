"use client";

import { useEffect, useMemo } from "react";
import { useTranslations } from "next-intl";
import { buildNodeMetadataSections } from "../../../../console/models";
import { useNetworkInterfaces } from "../../../../hooks/useNetworkInterfaces";
import { SummaryMetricCard, SystemPanelDrilldown } from "./SystemPanelDrilldown";
import { analyzeTelemetryDetails, isHot } from "./nodeMetricsModel";
import {
  CURATED_SECTIONS,
  buildDrilldownContent,
  formatBytes,
  formatPercent,
  formatRate,
  gatherMetrics,
  readMeta,
  relevantHistorySeries,
} from "./systemPanelModel";
import type { SystemPanelProps } from "./systemPanelTypes";

export type { SystemDrilldownView, SystemPanelProps } from "./systemPanelTypes";

/** The metadata section names the Hardware view translates; others keep the hub's own name. */
const SECTION_KEYS: Record<string, string> = {
  System: "system",
  Hardware: "hardware",
  CPU: "cpu",
  Memory: "memory",
  Storage: "storage",
  Firmware: "firmware",
  Network: "network",
};

const dash = (text: string) => (text === "" || text === "n/a" ? "—" : text);

/**
 * The System tab's Hardware view: CPU, memory, storage and network now as one sheet of four cells (each opens its
 * drill-down), then what the device reports about itself, section by section.
 */
export function SystemPanel({
  nodeId,
  asset,
  telemetry,
  telemetryDetails = null,
  telemetryLoading = false,
  telemetryWindow,
  drilldown = null,
  onOpenDrilldown,
  onCloseDrilldown,
  onOpenPanel,
}: SystemPanelProps) {
  const t = useTranslations("devices.hardware");
  const sections = buildNodeMetadataSections(asset.metadata);
  const curated = sections.filter((section) => CURATED_SECTIONS.includes(section.title));
  const metrics = gatherMetrics(asset, telemetry);
  const analyzedSeries = useMemo(() => analyzeTelemetryDetails(telemetryDetails), [telemetryDetails]);
  const historySeries = useMemo(
    () => (drilldown ? relevantHistorySeries(analyzedSeries, drilldown) : []),
    [analyzedSeries, drilldown]
  );
  const {
    interfaces,
    loading: interfacesLoading,
    error: interfacesError,
    refresh: refreshInterfaces,
  } = useNetworkInterfaces(nodeId);

  useEffect(() => {
    if (drilldown !== "network") {
      return;
    }
    void refreshInterfaces();
  }, [drilldown, refreshInterfaces]);

  if (drilldown) {
    return (
      <SystemPanelDrilldown
        content={buildDrilldownContent(asset, metrics, drilldown)}
        drilldown={drilldown}
        historySeries={historySeries}
        telemetryLoading={telemetryLoading}
        telemetryWindow={telemetryWindow}
        onOpenDrilldown={onOpenDrilldown}
        onCloseDrilldown={onCloseDrilldown}
        onOpenPanel={onOpenPanel}
        interfaces={interfaces}
        interfacesLoading={interfacesLoading}
        interfacesError={interfacesError}
      />
    );
  }

  if (curated.length === 0) {
    return <div className="dv-sheet dv-empty"><b>{t("empty")}</b></div>;
  }

  const interfaceCount = Number(readMeta(asset, "network_interface_count"));
  const rx = formatRate(metrics.network_rx_bytes_per_sec);
  const tx = formatRate(metrics.network_tx_bytes_per_sec);

  return (
    <div className="dmt">
      <section className="dv-sec" aria-label={t("usage")}>
        <div className="dmt-cells dmt-four">
          <SummaryMetricCard
            title={t("views.cpu")}
            value={dash(formatPercent(metrics.cpu_used_percent))}
            hot={isHot(metrics.cpu_used_percent, "percent")}
            hint={readMeta(asset, "cpu_model")}
            onClick={() => onOpenDrilldown?.("cpu")}
          />
          <SummaryMetricCard
            title={t("views.memory")}
            value={dash(formatPercent(metrics.memory_used_percent))}
            hot={isHot(metrics.memory_used_percent, "percent")}
            hint={formatBytes(readMeta(asset, "memory_total_bytes") || readMeta(asset, "physmem"))}
            onClick={() => onOpenDrilldown?.("memory")}
          />
          <SummaryMetricCard
            title={t("views.storage")}
            value={dash(formatPercent(metrics.disk_used_percent))}
            hot={isHot(metrics.disk_used_percent, "percent")}
            hint={formatBytes(readMeta(asset, "disk_root_total_bytes"))}
            onClick={() => onOpenDrilldown?.("storage")}
          />
          <SummaryMetricCard
            title={t("views.network")}
            value={rx === "n/a" && tx === "n/a" ? "—" : `${dash(rx)} / ${dash(tx)}`}
            hint={readMeta(asset, "network_interface_count") !== "" && Number.isFinite(interfaceCount)
              ? t("interfaces", { count: interfaceCount })
              : ""}
            onClick={() => onOpenDrilldown?.("network")}
          />
        </div>
      </section>

      {curated.map((section) => {
        const key = SECTION_KEYS[section.title];
        return (
          <section key={section.title} className="dv-sec" aria-labelledby={`dmt-s-${section.title}`}>
            <div className="dv-sh">
              <h2 id={`dmt-s-${section.title}`}>{key ? t(`metaSections.${key}`) : section.title}</h2>
            </div>
            <dl className="dv-sheet tight dv-kv dmt-kv2">
              {section.rows.map((row) => (
                <div key={row.key}>
                  <dt>{row.label}</dt>
                  <dd><span>{row.value}</span></dd>
                </div>
              ))}
            </dl>
          </section>
        );
      })}
    </div>
  );
}
