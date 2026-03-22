"use client";

import { formatMetric } from "../../../../console/formatters";
import type { AnalyzedSeries } from "./nodeMetricsModel";
import {
  buildAreaPath,
  buildLinePath,
  CHART_BOTTOM,
  CHART_TOP,
  flagToneClass,
  formatDurationSeconds,
  formatSignedMetric,
  formatTs,
  isCriticalThreshold,
  metricValueTone,
  thresholdBands,
  valueToChartY,
} from "./nodeMetricsModel";

export type SummaryTone = "neutral" | "ok" | "warn";

export function SummaryStat({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone: SummaryTone;
}) {
  const toneClass = tone === "ok"
    ? "border-[var(--ok)]/30 bg-[var(--ok-glow)] text-[var(--ok)]"
    : tone === "warn"
      ? "border-[var(--warn)]/30 bg-[var(--warn-glow)] text-[var(--warn)]"
      : "border-[var(--line)] bg-[var(--surface)] text-[var(--text)]";

  return (
    <div className={`rounded-md border px-2 py-1.5 ${toneClass}`}>
      <p className="text-[10px] uppercase tracking-wide text-[var(--muted)]">{label}</p>
      <p className="text-xs font-medium">{value}</p>
    </div>
  );
}

export function MetricPanel({ series }: { series: AnalyzedSeries }) {
  const valueTone = metricValueTone(series.current, series.unit);
  const currentValue = formatMetric(series.current ?? undefined, series.unit);
  const minValue = formatMetric(series.min, series.unit);
  const avgValue = formatMetric(series.avg, series.unit);
  const maxValue = formatMetric(series.max, series.unit);
  const trendValue = series.trendDelta == null ? "--" : formatSignedMetric(series.trendDelta, series.unit);
  const trendTone = series.trendDelta == null
    ? "text-[var(--muted)]"
    : series.trendDelta > 0
      ? "text-[var(--warn)]"
      : series.trendDelta < 0
        ? "text-[var(--ok)]"
        : "text-[var(--muted)]";
  const freshnessLabel = series.lagSeconds == null ? "Latest sample unavailable" : `Last sample ${formatDurationSeconds(series.lagSeconds)} ago`;
  const freshnessTone = series.lagSeconds != null && series.lagSeconds > 300 ? "text-[var(--warn)]" : "text-[var(--muted)]";

  return (
    <div className="space-y-3 rounded-lg border border-[var(--line)] p-3">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <h3 className="text-sm font-medium text-[var(--text)]">{series.label}</h3>
            {series.flags.map((flag, idx) => (
              <span
                key={`${series.metric}-flag-${idx}`}
                className={`rounded-full border px-1.5 py-0.5 text-[10px] ${flagToneClass(flag.tone)}`}
                title={flag.detail}
              >
                {flag.label}
              </span>
            ))}
          </div>
          <p className="text-xs text-[var(--muted)]">
            Min {minValue} · Avg {avgValue} · Max {maxValue} · {series.rawSampleCount} samples
          </p>
          <p className={`text-xs ${freshnessTone}`}>{freshnessLabel}</p>
        </div>
        <div className="text-right">
          <p className={`text-sm font-semibold ${valueTone}`}>{currentValue}</p>
          <p className="text-[10px] text-[var(--muted)]">Current</p>
          <p className={`mt-1 text-[10px] ${trendTone}`}>Trend {trendValue}</p>
        </div>
      </div>

      {series.points.length > 0 ? (
        <>
          <MetricChart series={series} />
          <div className="flex items-center justify-between text-[10px] text-[var(--muted)]">
            <span>{series.firstTs ? formatTs(series.firstTs) : "--"}</span>
            <span>{series.lastTs ? formatTs(series.lastTs) : "--"}</span>
          </div>
        </>
      ) : (
        <p className="text-xs text-[var(--muted)]">No data points in this window.</p>
      )}
    </div>
  );
}

function MetricChart({ series }: { series: AnalyzedSeries }) {
  const path = buildLinePath(series.points);
  const area = buildAreaPath(series.points);
  const lastPoint = series.points[series.points.length - 1];
  const chartTopLabel = formatMetric(series.yMax, series.unit);
  const chartBottomLabel = formatMetric(series.yMin, series.unit);
  const zoneBands = thresholdBands(series);

  return (
    <div className="space-y-2 rounded-md border border-[var(--line)] bg-[var(--surface)]/50 p-1.5">
      <div className="relative">
        <svg viewBox="0 0 100 100" className="h-28 w-full" role="img" aria-label={`${series.label} history`}>
          {zoneBands.map((band) => {
            const y1 = valueToChartY(band.high, series.yMin, series.yMax);
            const y2 = valueToChartY(band.low, series.yMin, series.yMax);
            const top = Math.min(y1, y2);
            const height = Math.max(Math.abs(y2 - y1), 0);
            if (height <= 0.15) return null;
            return (
              <rect
                key={`${series.metric}-band-${band.key}`}
                x={0}
                y={top}
                width={100}
                height={height}
                fill={band.color}
                opacity={band.opacity}
              />
            );
          })}

          {[10, 30, 50, 70, 90].map((y) => (
            <line key={y} x1={0} y1={y} x2={100} y2={y} stroke="var(--line)" strokeWidth={0.5} />
          ))}

          {series.thresholdLines.map((value) => {
            const y = valueToChartY(value, series.yMin, series.yMax);
            if (y < CHART_TOP || y > CHART_BOTTOM) return null;
            const critical = isCriticalThreshold(series.unit, value);
            return (
              <line
                key={`${series.metric}-threshold-${value}`}
                x1={0}
                y1={y}
                x2={100}
                y2={y}
                stroke={critical ? "var(--bad)" : "var(--warn)"}
                strokeWidth={0.65}
                strokeDasharray="2 1.5"
                opacity={0.75}
              />
            );
          })}

          <path d={area} fill="var(--accent)" opacity={0.14} />
          <path d={path} fill="none" stroke="var(--accent)" strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round" />

          {lastPoint ? (
            <circle
              cx={lastPoint.x}
              cy={lastPoint.y}
              r={1.8}
              fill="var(--accent)"
              stroke="var(--surface)"
              strokeWidth={0.5}
            />
          ) : null}
        </svg>
        <div className="pointer-events-none absolute inset-y-1 right-1.5 flex flex-col justify-between text-[10px] text-[var(--muted)]">
          <span>{chartTopLabel}</span>
          <span>{chartBottomLabel}</span>
        </div>
      </div>
      {series.thresholdLines.length > 0 ? (
        <div className="flex flex-wrap gap-2">
          {series.thresholdLines.map((value) => (
            <span
              key={`${series.metric}-legend-${value}`}
              className={`rounded-full border px-1.5 py-0.5 text-[10px] ${
                isCriticalThreshold(series.unit, value)
                  ? "border-[var(--bad)]/40 text-[var(--bad)] bg-[var(--bad-glow)]"
                  : "border-[var(--warn)]/40 text-[var(--warn)] bg-[var(--warn-glow)]"
              }`}
            >
              {isCriticalThreshold(series.unit, value) ? "Critical" : "Warning"} at {formatMetric(value, series.unit)}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  );
}
