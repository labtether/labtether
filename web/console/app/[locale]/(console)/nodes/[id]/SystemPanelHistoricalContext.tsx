"use client";

import { useTranslations } from "next-intl";
import type { TelemetryWindow } from "../../../../console/models";
import type { AnalyzedSeries } from "./nodeMetricsModel";
import { MetricPanel } from "./NodeMetricsPanels";

/** The drill-down's history: one chart cell per metric tied to it (CPU and temperature, memory, disk, in and out). */
export function SystemPanelHistoricalContext({
  series,
  telemetryLoading,
  telemetryWindow,
}: {
  series: AnalyzedSeries[];
  telemetryLoading: boolean;
  telemetryWindow?: TelemetryWindow;
}) {
  const t = useTranslations("devices.hardware");
  if (telemetryLoading && series.length === 0) {
    return <div className="dv-sheet dv-empty">{t("historyLoading")}</div>;
  }
  if (series.length === 0) {
    return (
      <div className="dv-sheet dv-empty">
        <b>{telemetryWindow ? t("historyNoneIn", { window: telemetryWindow }) : t("historyNone")}</b>
      </div>
    );
  }
  return (
    <div className={`dmt-cells${series.length === 1 ? " one" : ""}`}>
      {series.map((entry) => (
        <MetricPanel key={entry.metric} series={entry} />
      ))}
    </div>
  );
}
