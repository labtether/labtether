"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { metricWindows } from "../../../../console/models";
import type {
  AssetTelemetryDetails,
  MetricWindow,
} from "../../../../console/models";
import { NodeMetricsControls } from "./NodeMetricsControls";
import { MetricPanel } from "./NodeMetricsPanels";
import {
  analyzeTelemetryDetails,
  formatRange,
  formatStep,
} from "./nodeMetricsModel";
import { useMetricWords } from "./metricsViewWords";

type Props = {
  telemetryDetails: AssetTelemetryDetails | null;
  telemetryLoading: boolean;
  telemetryWindow: MetricWindow;
  onTelemetryWindowChange: (window: MetricWindow) => void;
  requestedFocusMetric?: string | null;
};

/**
 * The System tab's Metrics view: every metric the device reports over the chosen range, one chart each. The bar
 * holds the range, the time span and one metric to show alone; chips below show or hide metrics.
 */
export function NodeMetricsTab({
  telemetryDetails,
  telemetryLoading,
  telemetryWindow,
  onTelemetryWindowChange,
  requestedFocusMetric,
}: Props) {
  const t = useTranslations("devices.metrics");
  const words = useMetricWords();
  const analyzed = useMemo(
    () => analyzeTelemetryDetails(telemetryDetails),
    [telemetryDetails]
  );

  const [hiddenMetrics, setHiddenMetrics] = useState<Record<string, boolean>>({});
  const [focusMetric, setFocusMetric] = useState<string | null>(null);

  const metricSignature = useMemo(
    () => analyzed.map((series) => series.metric).join("|"),
    [analyzed]
  );

  useEffect(() => {
    setHiddenMetrics((prev) => {
      const next: Record<string, boolean> = {};
      for (const series of analyzed) {
        next[series.metric] = prev[series.metric] ?? false;
      }
      return next;
    });
  }, [metricSignature, analyzed]);

  const visibleSeries = useMemo(
    () => analyzed.filter((series) => !hiddenMetrics[series.metric]),
    [analyzed, hiddenMetrics]
  );

  useEffect(() => {
    if (!focusMetric) return;
    const stillVisible = visibleSeries.some((series) => series.metric === focusMetric);
    if (!stillVisible) {
      setFocusMetric(null);
    }
  }, [focusMetric, visibleSeries]);

  useEffect(() => {
    const requested = requestedFocusMetric?.trim();
    if (!requested) return;
    const exists = analyzed.some((series) => series.metric === requested);
    if (!exists) return;
    setHiddenMetrics((prev) => {
      if (!prev[requested]) return prev;
      return { ...prev, [requested]: false };
    });
    setFocusMetric((prev) => (prev === requested ? prev : requested));
  }, [requestedFocusMetric, analyzed]);

  const displayedSeries = useMemo(() => {
    if (!focusMetric) return visibleSeries;
    return visibleSeries.filter((series) => series.metric === focusMetric);
  }, [focusMetric, visibleSeries]);

  const problemCount = analyzed.filter((series) => series.warningCount > 0).length;
  const hiddenCount = analyzed.length - visibleSeries.length;
  const span = telemetryDetails ? formatRange(telemetryDetails.from, telemetryDetails.to) : "";
  const showAll = () =>
    setHiddenMetrics((prev) => Object.fromEntries(Object.keys(prev).map((metric) => [metric, false])));

  return (
    <div className="dmt">
      <div className="dv-bar">
        <div className="dv-seg" role="group" aria-label={t("window")}>
          {metricWindows.map((window) => (
            <button
              key={window}
              type="button"
              aria-pressed={window === telemetryWindow}
              onClick={() => onTelemetryWindowChange(window)}
            >
              {window}
            </button>
          ))}
        </div>
        {telemetryDetails ? (
          <span className="dv-hint">
            {[span, telemetryDetails.step ? t("every", { step: formatStep(telemetryDetails.step) }) : ""].filter(Boolean).join(" · ")}
            {problemCount > 0 ? <span className="dmt-warn"> · {t("withProblems", { count: problemCount })}</span> : null}
          </span>
        ) : null}
        <span className="dv-sp" />
        {visibleSeries.length > 1 ? (
          <select
            className="dv-input dmt-select"
            aria-label={t("focus")}
            value={focusMetric ?? ""}
            onChange={(event) => setFocusMetric(event.target.value || null)}
          >
            <option value="">{t("allMetrics")}</option>
            {visibleSeries.map((series) => (
              <option key={series.metric} value={series.metric}>{words.name(series)}</option>
            ))}
          </select>
        ) : null}
      </div>

      {telemetryLoading ? (
        <p className="dv-hint">{t("loading")}</p>
      ) : analyzed.length === 0 ? (
        <div className="dv-sheet dv-empty"><b>{t("empty")}</b></div>
      ) : (
        <>
          <NodeMetricsControls
            analyzed={analyzed}
            hiddenMetrics={hiddenMetrics}
            hiddenCount={hiddenCount}
            onShowAllMetrics={showAll}
            onToggleMetric={(metric) => setHiddenMetrics((prev) => ({ ...prev, [metric]: !prev[metric] }))}
          />
          {displayedSeries.length === 0 ? (
            <div className="dv-sheet dv-empty dmt-gap">
              <b>{t("allHidden")}</b>
              <button type="button" className="dmt-quiet dv-empty-act" onClick={showAll}>{t("showAll", { count: hiddenCount })}</button>
            </div>
          ) : (
            <div className={`dmt-cells dmt-gap${displayedSeries.length === 1 ? " one" : ""}`}>
              {displayedSeries.map((series) => (
                <MetricPanel key={series.metric} series={series} />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}
