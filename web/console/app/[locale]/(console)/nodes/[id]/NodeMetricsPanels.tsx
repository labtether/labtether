"use client";

import { useTranslations } from "next-intl";
import type { AnalyzedSeries, QualityFlag } from "./nodeMetricsModel";
import {
  formatDurationSeconds,
  formatReadout,
  formatSignedMetric,
  isHot,
} from "./nodeMetricsModel";
import { MetricChart } from "./metricsViewChart";
import { useMetricWords } from "./metricsViewWords";

/** Data-quality words for a metric (Stale, Gaps …): amber or red when they matter, quiet otherwise. */
export function MetricFlags({ flags }: { flags: QualityFlag[] }) {
  const words = useMetricWords();
  if (flags.length === 0) return null;
  return (
    <span className="dmt-flags">
      {flags.map((flag) => (
        <span key={flag.kind} className={`dmt-flag ${flag.tone}`} title={words.flagDetail(flag)}>
          {words.flag(flag)}
        </span>
      ))}
    </span>
  );
}

/**
 * One metric in the chart sheet: its name and any data-quality words, the latest value with the range's low,
 * average and high beside it, the chart, then how fresh the last sample is.
 */
export function MetricPanel({ series }: { series: AnalyzedSeries }) {
  const t = useTranslations("devices.metrics");
  const words = useMetricWords();
  const unit = series.unit;
  const hasPoints = series.points.length > 0;
  const stale = series.lagSeconds != null && series.lagSeconds > 300;
  const facts = [
    t("samples", { count: series.rawSampleCount }),
    series.trendDelta == null ? "" : t("trend", { value: formatSignedMetric(series.trendDelta, unit) }),
  ].filter(Boolean);

  return (
    <div className="dmt-cell">
      <div className="dmt-cell-h">
        <h3 className="dmt-k">{words.name(series)}</h3>
        <MetricFlags flags={series.flags} />
      </div>
      <div className="dmt-read">
        <b className={`dmt-readout${isHot(series.current, unit) ? " hot" : ""}`}>{formatReadout(series.current, unit)}</b>
        {hasPoints ? (
          <span className="dmt-u">
            {t("range", { min: formatReadout(series.min, unit), avg: formatReadout(series.avg, unit), max: formatReadout(series.max, unit) })}
          </span>
        ) : null}
      </div>
      {hasPoints ? <MetricChart series={series} /> : <p className="dv-hint dmt-few">{t("noSamples")}</p>}
      <p className="dmt-foot">
        <span className={stale ? "dmt-warn" : ""}>
          {series.lagSeconds == null ? t("noLastSample") : t("lastSample", { age: formatDurationSeconds(series.lagSeconds) })}
        </span>
        {facts.map((fact) => (
          <span key={fact}>{fact}</span>
        ))}
      </p>
    </div>
  );
}
