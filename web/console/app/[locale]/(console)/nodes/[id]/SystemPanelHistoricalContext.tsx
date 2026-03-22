"use client";

import { formatMetric } from "../../../../console/formatters";
import type { TelemetryWindow } from "../../../../console/models";
import { Card } from "../../../../components/ui/Card";
import { buildLinePath } from "./nodeMetricsChartGeometry";
import type { AnalyzedSeries } from "./nodeMetricsModel";
import {
  formatDurationSeconds,
  formatSignedMetric,
  metricValueTone,
} from "./nodeMetricsPresentationModel";

function HistoryStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-[var(--line)]/70 bg-[var(--panel-glass)] px-2 py-1">
      <p className="text-[10px] uppercase tracking-wide text-[var(--muted)]">{label}</p>
      <p className="text-xs font-medium tabular-nums text-[var(--text)]">{value}</p>
    </div>
  );
}

function HistoryMetricCard({ series }: { series: AnalyzedSeries }) {
  const linePath = buildLinePath(series.points);
  const currentToneClass = metricValueTone(series.current, series.unit);
  const trendLabel =
    series.trendDelta == null ? "--" : formatSignedMetric(series.trendDelta, series.unit);
  const freshnessLabel =
    series.lagSeconds == null
      ? "Latest sample unavailable"
      : `Last sample ${formatDurationSeconds(series.lagSeconds)} ago`;

  return (
    <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-[var(--text)]">{series.label}</p>
          <p className="mt-1 text-[11px] text-[var(--muted)]">{freshnessLabel}</p>
        </div>
        <div className="text-right">
          <p className={`text-sm font-semibold tabular-nums ${currentToneClass}`}>
            {formatMetric(series.current ?? undefined, series.unit)}
          </p>
          <p className="text-[10px] text-[var(--muted)]">Current</p>
        </div>
      </div>

      <div className="mt-3 rounded-md border border-[var(--line)]/80 bg-[var(--panel-glass)] px-2 py-1.5">
        {series.points.length > 1 ? (
          <svg
            viewBox="0 0 100 100"
            className="h-20 w-full"
            role="img"
            aria-label={`${series.label} sparkline`}
          >
            <defs>
              <linearGradient id={`spark-${series.metric}`} x1="0" x2="0" y1="0" y2="1">
                <stop offset="0%" stopColor="var(--accent)" stopOpacity="0.28" />
                <stop offset="100%" stopColor="var(--accent)" stopOpacity="0.04" />
              </linearGradient>
            </defs>
            <path d={`${linePath} L 100 92 L 0 92 Z`} fill={`url(#spark-${series.metric})`} />
            <path
              d={linePath}
              fill="none"
              stroke="var(--accent)"
              strokeWidth={2}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        ) : (
          <div className="flex h-20 items-center justify-center text-xs text-[var(--muted)]">
            More samples needed for trend rendering.
          </div>
        )}
      </div>

      <div className="mt-3 grid grid-cols-4 gap-2">
        <HistoryStat label="Min" value={formatMetric(series.min, series.unit)} />
        <HistoryStat label="Avg" value={formatMetric(series.avg, series.unit)} />
        <HistoryStat label="Max" value={formatMetric(series.max, series.unit)} />
        <HistoryStat label="Trend" value={trendLabel} />
      </div>

      {series.flags.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-1.5">
          {series.flags.map((flag, index) => (
            <span
              key={`${series.metric}-flag-${index}`}
              className="rounded-full border border-[var(--line)] bg-[var(--panel-glass)] px-2 py-0.5 text-[10px] text-[var(--muted)]"
              title={flag.detail}
            >
              {flag.label}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  );
}

export function SystemPanelHistoricalContext({
  series,
  telemetryLoading,
  telemetryWindow,
}: {
  series: AnalyzedSeries[];
  telemetryLoading: boolean;
  telemetryWindow?: TelemetryWindow;
}) {
  return (
    <Card>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h4 className="text-sm font-medium text-[var(--text)]">Historical Context</h4>
          <p className="text-xs text-[var(--muted)]">
            {telemetryWindow
              ? `Last ${telemetryWindow}`
              : "Recent telemetry window"}{" "}
            for the signals tied to this drilldown.
          </p>
        </div>
      </div>

      {telemetryLoading && series.length === 0 ? (
        <div className="mt-4 rounded-lg border border-dashed border-[var(--line)] p-4 text-sm text-[var(--muted)]">
          Loading telemetry history...
        </div>
      ) : series.length > 0 ? (
        <div className="mt-4 grid grid-cols-1 gap-3 xl:grid-cols-2">
          {series.map((entry) => (
            <HistoryMetricCard key={entry.metric} series={entry} />
          ))}
        </div>
      ) : (
        <div className="mt-4 rounded-lg border border-dashed border-[var(--line)] p-4 text-sm text-[var(--muted)]">
          No historical telemetry is available for this drilldown yet. Use Metrics if you want the full panel once samples start arriving.
        </div>
      )}
    </Card>
  );
}
