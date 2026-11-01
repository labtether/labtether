"use client";

import { useTranslations } from "next-intl";
import type { AnalyzedSeries, QualityFlag } from "./nodeMetricsModel";
import { formatDurationSeconds } from "./nodeMetricsModel";

/** Translated words for metric charts: a metric's name, a data-quality flag and its tooltip. */
export function useMetricWords() {
  const t = useTranslations("devices.metrics");
  return {
    /** "CPU", "Network in"; the model's own label for a metric without a translation. */
    name: (series: Pick<AnalyzedSeries, "metric" | "label">) =>
      t.has(`names.${series.metric}`) ? t(`names.${series.metric}`) : series.label,
    flag: (flag: QualityFlag) => t(`flags.${flag.kind}`),
    flagDetail: (flag: QualityFlag) =>
      t(`flagDetails.${flag.kind}`, { age: formatDurationSeconds(Math.max(0, Math.round(flag.seconds ?? 0))) }),
    chartLabel: (series: Pick<AnalyzedSeries, "metric" | "label">) =>
      t("chartLabel", { metric: t.has(`names.${series.metric}`) ? t(`names.${series.metric}`) : series.label }),
    tooFew: () => t("tooFew"),
  };
}
