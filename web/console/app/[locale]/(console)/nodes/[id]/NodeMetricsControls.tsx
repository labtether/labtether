"use client";

import { formatMetric } from "../../../../console/formatters";
import type { AnalyzedSeries } from "./nodeMetricsModel";
import { dotToneClass, metricValueTone } from "./nodeMetricsModel";

type NodeMetricsControlsProps = {
  analyzed: AnalyzedSeries[];
  visibleSeries: AnalyzedSeries[];
  hiddenMetrics: Record<string, boolean>;
  focusMetric: string | null;
  hiddenCount: number;
  selectedLabel: string;
  onShowAllMetrics: () => void;
  onToggleMetric: (metric: string) => void;
  onFocusMetricChange: (metric: string | null) => void;
};

export function NodeMetricsControls({
  analyzed,
  visibleSeries,
  hiddenMetrics,
  focusMetric,
  hiddenCount,
  selectedLabel,
  onShowAllMetrics,
  onToggleMetric,
  onFocusMetricChange,
}: NodeMetricsControlsProps) {
  return (
    <>
      <div className="space-y-2">
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs uppercase tracking-wide text-[var(--muted)]">Visible Metrics</p>
          {hiddenCount > 0 ? (
            <button
              type="button"
              className="rounded-md border border-[var(--line)] px-2 py-1 text-xs text-[var(--muted)] hover:text-[var(--text)]"
              onClick={onShowAllMetrics}
            >
              Show all ({hiddenCount} hidden)
            </button>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          {analyzed.map((series) => {
            const hidden = hiddenMetrics[series.metric];
            const valueTone = metricValueTone(series.current, series.unit);
            const valueColor = valueTone === "text-[var(--bad)]"
              ? "text-[var(--bad)]"
              : valueTone === "text-[var(--warn)]"
                ? "text-[var(--warn)]"
                : valueTone === "text-[var(--ok)]"
                  ? "text-[var(--ok)]"
                  : "text-[var(--muted)]";

            return (
              <button
                key={series.metric}
                type="button"
                aria-pressed={!hidden}
                className={`rounded-md border px-2 py-1 text-xs transition-colors ${
                  hidden
                    ? "border-[var(--line)] text-[var(--muted)] bg-transparent opacity-75"
                    : "border-[var(--line)] text-[var(--text)] bg-[var(--surface)]"
                }`}
                onClick={() => onToggleMetric(series.metric)}
                title={hidden ? "Show metric" : "Hide metric"}
              >
                <span className="inline-flex items-center gap-1 font-medium">
                  <span className={`h-1.5 w-1.5 rounded-full ${dotToneClass(series.current, series.unit)}`} />
                  {series.label}
                </span>
                <span className={`ml-2 ${valueColor}`}>{formatMetric(series.current ?? undefined, series.unit)}</span>
                {series.warningCount > 0 ? (
                  <span className="ml-1.5 rounded-full border border-[var(--warn)]/30 bg-[var(--warn-glow)] px-1 py-0.5 text-[10px] text-[var(--warn)]">
                    {series.warningCount}
                  </span>
                ) : null}
              </button>
            );
          })}
        </div>
      </div>

      {visibleSeries.length > 0 ? (
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs uppercase tracking-wide text-[var(--muted)]">Focus Mode</p>
            <p className="text-xs text-[var(--muted)]">{selectedLabel}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              className={`rounded-md border px-2 py-1 text-xs transition-colors ${
                focusMetric === null
                  ? "border-[var(--control-bg-active)] bg-[var(--control-bg-active)] text-[var(--control-fg-active)]"
                  : "border-[var(--line)] text-[var(--muted)] hover:text-[var(--text)]"
              }`}
              onClick={() => onFocusMetricChange(null)}
            >
              All visible metrics
            </button>
            {visibleSeries.map((series) => (
              <button
                key={series.metric}
                type="button"
                className={`rounded-md border px-2 py-1 text-xs transition-colors ${
                  focusMetric === series.metric
                    ? "border-[var(--control-bg-active)] bg-[var(--control-bg-active)] text-[var(--control-fg-active)]"
                    : "border-[var(--line)] text-[var(--muted)] hover:text-[var(--text)]"
                }`}
                onClick={() => onFocusMetricChange(series.metric)}
              >
                {series.label}
              </button>
            ))}
          </div>
        </div>
      ) : null}
    </>
  );
}
