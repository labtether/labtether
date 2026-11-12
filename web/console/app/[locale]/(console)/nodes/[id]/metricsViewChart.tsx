"use client";

import type { CSSProperties } from "react";
import type { AnalyzedSeries } from "./nodeMetricsModel";
import {
  buildSmoothAreaPath,
  buildSmoothPath,
  CHART_BOTTOM,
  CHART_TOP,
  formatAxisValue,
  isHot,
} from "./nodeMetricsModel";
import { timeTicks } from "./metricsTimeTicks";
import { useMetricWords } from "./metricsViewWords";

const MID = (CHART_TOP + CHART_BOTTOM) / 2;

/**
 * One metric over time: a smooth line in the chart hue with a flat fill when it moves, dashed gridlines with the
 * scale on the right and times underneath. The line turns amber when the latest value is at or above the hot mark.
 */
export function MetricChart({ series, second, small = false }: { series: AnalyzedSeries; second?: AnalyzedSeries; small?: boolean }) {
  const words = useMetricWords();
  if (series.points.length < 2) {
    return <p className="dv-hint dmt-few">{words.tooFew()}</p>;
  }
  const hot = isHot(series.current, series.unit);
  const moves = series.max - series.min > 0;
  const ticks = timeTicks(series.points);
  const pos = (y: number): CSSProperties => ({ top: `${y}%` });

  return (
    <div className={`dmt-chart${hot ? " hot" : ""}${small ? " sm" : ""}`}>
      <div className="dmt-plot">
        <svg viewBox="0 0 100 100" preserveAspectRatio="none" role="img" aria-label={words.chartLabel(series)}>
          <line className="dmt-grid" x1={0} x2={100} y1={CHART_TOP} y2={CHART_TOP} />
          <line className="dmt-grid" x1={0} x2={100} y1={MID} y2={MID} />
          <line className="dmt-base" x1={0} x2={100} y1={CHART_BOTTOM} y2={CHART_BOTTOM} />
          {moves ? <path className="dmt-fill" d={buildSmoothAreaPath(series.points)} /> : null}
          <path className="dmt-line" d={buildSmoothPath(series.points)} />
          {second && second.points.length > 1 ? <path className="dmt-line dmt-line2" d={buildSmoothPath(second.points)} /> : null}
        </svg>
        <span className="dmt-y" style={pos(CHART_TOP)}>{formatAxisValue(series.yMax, series.unit)}</span>
        <span className="dmt-y" style={pos(MID)}>{formatAxisValue((series.yMax + series.yMin) / 2, series.unit)}</span>
        <span className="dmt-y" style={pos(CHART_BOTTOM)}>{formatAxisValue(series.yMin, series.unit)}</span>
      </div>
      <div className="dmt-x" aria-hidden="true">
        {ticks.map((tick) => (
          <span
            key={tick.label}
            className={tick.x < 6 ? "first" : tick.x > 94 ? "last" : ""}
            style={{ left: `${tick.x}%` }}
          >
            {tick.label}
          </span>
        ))}
      </div>
    </div>
  );
}
