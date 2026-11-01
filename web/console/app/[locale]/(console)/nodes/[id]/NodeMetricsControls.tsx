"use client";

import { useTranslations } from "next-intl";
import type { AnalyzedSeries } from "./nodeMetricsModel";
import { formatReadout, isHot } from "./nodeMetricsModel";
import { useMetricWords } from "./metricsViewWords";

type NodeMetricsControlsProps = {
  analyzed: AnalyzedSeries[];
  hiddenMetrics: Record<string, boolean>;
  hiddenCount: number;
  onShowAllMetrics: () => void;
  onToggleMetric: (metric: string) => void;
};

/**
 * Which metrics the chart sheet shows: one pressed chip per metric with its latest value (a dot when its data has
 * problems), and Show all while any is hidden.
 */
export function NodeMetricsControls({
  analyzed,
  hiddenMetrics,
  hiddenCount,
  onShowAllMetrics,
  onToggleMetric,
}: NodeMetricsControlsProps) {
  const t = useTranslations("devices.metrics");
  const words = useMetricWords();
  return (
    <div className="dmt-chips" role="group" aria-label={t("shown")}>
      {analyzed.map((series) => {
        const shown = !hiddenMetrics[series.metric];
        return (
          <button
            key={series.metric}
            type="button"
            className="dmt-chip"
            aria-pressed={shown}
            onClick={() => onToggleMetric(series.metric)}
          >
            {series.warningCount > 0 ? <span className="dv-dot warn" aria-label={t("hasProblems")} /> : null}
            {words.name(series)}
            <span className={`dmt-chip-v${isHot(series.current, series.unit) ? " hot" : ""}`}>
              {formatReadout(series.current, series.unit)}
            </span>
          </button>
        );
      })}
      {hiddenCount > 0 ? (
        <button type="button" className="dmt-quiet" onClick={onShowAllMetrics}>
          {t("showAll", { count: hiddenCount })}
        </button>
      ) : null}
    </div>
  );
}
