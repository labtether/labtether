import type {
  AssetTelemetryDetails,
  MetricPoint,
  TelemetrySeries,
} from "../../../../console/models";
import type { ChartPoint } from "./nodeMetricsChartGeometry";
import { valueToChartY } from "./nodeMetricsChartGeometry";
import { formatDurationSeconds } from "./nodeMetricsPresentationModel";
import type { QualityTone } from "./nodeMetricsPresentationModel";

export type QualityFlag = {
  label: string;
  tone: QualityTone;
  detail: string;
};

export type AnalyzedSeries = {
  metric: string;
  label: string;
  unit: string;
  current: number | null;
  min: number;
  max: number;
  avg: number;
  yMin: number;
  yMax: number;
  points: ChartPoint[];
  rawSampleCount: number;
  firstTs: number | null;
  lastTs: number | null;
  lagSeconds: number | null;
  trendDelta: number | null;
  flags: QualityFlag[];
  warningCount: number;
  thresholdLines: number[];
};

const METRIC_LABELS: Readonly<Record<string, string>> = {
  cpu_used_percent: "CPU Usage",
  memory_used_percent: "Memory Usage",
  disk_used_percent: "Disk Usage",
  temperature_celsius: "Temperature",
  network_rx_bytes_per_sec: "Network RX",
  network_tx_bytes_per_sec: "Network TX",
};

const METRIC_ORDER: Readonly<Record<string, number>> = {
  cpu_used_percent: 10,
  memory_used_percent: 20,
  disk_used_percent: 30,
  temperature_celsius: 40,
  network_rx_bytes_per_sec: 50,
  network_tx_bytes_per_sec: 60,
};

export function analyzeTelemetryDetails(
  telemetryDetails: AssetTelemetryDetails | null,
): AnalyzedSeries[] {
  const seriesList = normalizeTelemetrySeriesList(telemetryDetails?.series);
  if (!telemetryDetails || seriesList.length === 0) {
    return [];
  }

  const latestTsAcrossSeries = seriesList.reduce((latest, series) => {
    const ts = series.points.reduce(
      (seriesLatest, point) => Math.max(seriesLatest, point.ts),
      0,
    );
    return Math.max(latest, ts);
  }, 0);

  const toTs = Math.floor(new Date(telemetryDetails.to).getTime() / 1000);
  const windowEndTs =
    Number.isFinite(toTs) && toTs > 0 ? toTs : latestTsAcrossSeries;
  const stepSeconds = parseDurationSeconds(
    typeof telemetryDetails.step === "string" ? telemetryDetails.step : "",
  );

  return [...seriesList]
    .sort(
      (left, right) => metricSortWeight(left.metric) - metricSortWeight(right.metric),
    )
    .map((series) =>
      analyzeSeries(series, latestTsAcrossSeries, windowEndTs, stepSeconds),
    );
}

function normalizeTelemetrySeriesList(rawSeries: unknown): TelemetrySeries[] {
  if (!Array.isArray(rawSeries)) {
    return [];
  }

  const normalized: TelemetrySeries[] = [];
  for (const entry of rawSeries) {
    if (!entry || typeof entry !== "object") {
      continue;
    }

    const candidate = entry as {
      metric?: unknown;
      unit?: unknown;
      points?: unknown;
      current?: unknown;
    };

    const metric =
      typeof candidate.metric === "string" ? candidate.metric.trim() : "";
    if (!metric) {
      continue;
    }

    normalized.push({
      metric,
      unit: typeof candidate.unit === "string" ? candidate.unit.trim() : "",
      points: normalizeMetricPoints(candidate.points),
      current:
        typeof candidate.current === "number" &&
        Number.isFinite(candidate.current)
          ? candidate.current
          : undefined,
    });
  }

  return normalized;
}

function normalizeMetricPoints(rawPoints: unknown): MetricPoint[] {
  if (!Array.isArray(rawPoints)) {
    return [];
  }

  const normalized: MetricPoint[] = [];
  for (const entry of rawPoints) {
    if (!entry || typeof entry !== "object") {
      continue;
    }

    const candidate = entry as {
      ts?: unknown;
      value?: unknown;
    };

    const ts = normalizeFiniteNumber(candidate.ts);
    const value = normalizeFiniteNumber(candidate.value);
    if (ts == null || value == null) {
      continue;
    }

    normalized.push({ ts, value });
  }

  return normalized;
}

function normalizeFiniteNumber(value: unknown): number | null {
  if (typeof value === "number") {
    return Number.isFinite(value) ? value : null;
  }
  if (typeof value === "string" && value.trim() !== "") {
    const numeric = Number(value);
    return Number.isFinite(numeric) ? numeric : null;
  }
  return null;
}

function analyzeSeries(
  series: TelemetrySeries,
  latestTsAcrossSeries: number,
  windowEndTs: number,
  stepSeconds: number,
): AnalyzedSeries {
  const rawPoints = [...series.points]
    .filter((point) => Number.isFinite(point.value) && Number.isFinite(point.ts))
    .sort((left, right) => left.ts - right.ts);

  const values = rawPoints.map((point) => point.value);
  const min = values.length > 0 ? Math.min(...values) : 0;
  const max = values.length > 0 ? Math.max(...values) : 0;
  const avg =
    values.length > 0
      ? values.reduce((acc, value) => acc + value, 0) / values.length
      : 0;
  const current =
    typeof series.current === "number"
      ? series.current
      : rawPoints.length > 0
        ? rawPoints[rawPoints.length - 1].value
        : null;

  let yMin = min;
  let yMax = max;
  if (series.unit === "percent") {
    yMin = 0;
    yMax = Math.max(100, max * 1.05);
  } else if (min >= 0) {
    const span = Math.max(max - min, max * 0.15, 1);
    yMin = Math.max(0, min - span * 0.2);
    yMax = max + span * 0.2;
  } else {
    const span = Math.max(
      max - min,
      Math.max(Math.abs(min), Math.abs(max), 1) * 0.15,
    );
    yMin = min - span * 0.2;
    yMax = max + span * 0.2;
  }
  if (Math.abs(yMax - yMin) < 1e-6) {
    const bump = Math.max(Math.abs(yMax) * 0.1, 1);
    yMin -= bump;
    yMax += bump;
  }

  const sampled = downSample(rawPoints, 90);
  const points = sampled.map((point, idx) => {
    const x = sampled.length <= 1 ? 50 : (idx / (sampled.length - 1)) * 100;
    return {
      x,
      y: valueToChartY(point.value, yMin, yMax),
      ts: point.ts,
      value: point.value,
    };
  });

  const flags: QualityFlag[] = [];
  if (rawPoints.length < 4) {
    flags.push({
      label: "Sparse",
      tone: "info",
      detail: "Low sample count in this time window.",
    });
  }

  const lastTs = rawPoints.length > 0 ? rawPoints[rawPoints.length - 1].ts : null;
  const firstTs = rawPoints.length > 0 ? rawPoints[0].ts : null;
  const lagSeconds =
    lastTs == null
      ? null
      : Math.max(0, Math.round((windowEndTs || latestTsAcrossSeries) - lastTs));
  const trendDelta =
    rawPoints.length >= 2
      ? rawPoints[rawPoints.length - 1].value - rawPoints[0].value
      : null;

  if (lastTs != null && stepSeconds > 0) {
    const streamLagSeconds = latestTsAcrossSeries - lastTs;
    if (streamLagSeconds > stepSeconds * 3) {
      flags.push({
        label: "Stale",
        tone: streamLagSeconds > stepSeconds * 6 ? "bad" : "warn",
        detail: `Latest sample is ${formatDurationSeconds(streamLagSeconds)} behind.`,
      });
    }
  }

  const range = max - min;
  const flatTolerance =
    series.unit === "percent"
      ? 0.35
      : series.unit === "celsius"
        ? 0.2
        : Math.max(Math.abs(avg) * 0.01, 0.05);
  if (rawPoints.length >= 8 && range <= flatTolerance) {
    flags.push({
      label: "Flatline",
      tone: "info",
      detail: "Signal has negligible variance across the selected window.",
    });
  }

  if (rawPoints.length >= 10) {
    const deltas: number[] = [];
    for (let idx = 1; idx < values.length; idx += 1) {
      deltas.push(Math.abs(values[idx] - values[idx - 1]));
    }
    const avgDelta =
      deltas.length > 0
        ? deltas.reduce((acc, value) => acc + value, 0) / deltas.length
        : 0;
    if (range > 0 && avgDelta / range > 0.65) {
      flags.push({
        label: "Noisy",
        tone: "warn",
        detail: "Step-to-step variance is high relative to total range.",
      });
    }
    if (hasRobustOutlier(values)) {
      flags.push({
        label: "Outlier",
        tone: "warn",
        detail: "One or more spikes are well outside typical values.",
      });
    }
  }

  if (stepSeconds > 0 && rawPoints.length >= 2) {
    let largestGap = 0;
    for (let idx = 1; idx < rawPoints.length; idx += 1) {
      largestGap = Math.max(largestGap, rawPoints[idx].ts - rawPoints[idx - 1].ts);
    }
    if (largestGap > stepSeconds * 3) {
      flags.push({
        label: "Gaps",
        tone: largestGap > stepSeconds * 6 ? "bad" : "warn",
        detail: `Largest collection gap is ${formatDurationSeconds(largestGap)}.`,
      });
    }
  }

  const warningCount = flags.filter(
    (flag) => flag.tone === "warn" || flag.tone === "bad",
  ).length;

  return {
    metric: series.metric,
    label: metricLabel(series.metric),
    unit: series.unit,
    current,
    min,
    max,
    avg,
    yMin,
    yMax,
    points,
    rawSampleCount: rawPoints.length,
    firstTs,
    lastTs,
    lagSeconds,
    trendDelta,
    flags,
    warningCount,
    thresholdLines: thresholdLinesForUnit(series.unit),
  };
}

function thresholdLinesForUnit(unit: string): number[] {
  if (unit === "percent") return [70, 90];
  if (unit === "celsius") return [70, 80];
  return [];
}

function parseDurationSeconds(value: string): number {
  const trimmed = value.trim().toLowerCase();
  const match = trimmed.match(/^(\d+)([smhd])$/);
  if (!match) return 0;
  const count = Number.parseInt(match[1], 10);
  const unit = match[2];
  if (!Number.isFinite(count)) return 0;
  if (unit === "s") return count;
  if (unit === "m") return count * 60;
  if (unit === "h") return count * 3600;
  return count * 86400;
}

function downSample<T>(items: T[], maxCount: number): T[] {
  if (items.length <= maxCount) return items;
  const step = Math.ceil(items.length / maxCount);
  const sampled: T[] = [];
  for (let idx = 0; idx < items.length; idx += step) {
    sampled.push(items[idx]);
  }
  if (sampled[sampled.length - 1] !== items[items.length - 1]) {
    sampled.push(items[items.length - 1]);
  }
  return sampled;
}

function metricLabel(metric: string): string {
  return (
    METRIC_LABELS[metric] ??
    metric
      .replace(/[_-]+/g, " ")
      .replace(/\b\w/g, (match) => match.toUpperCase())
  );
}

function metricSortWeight(metric: string): number {
  return METRIC_ORDER[metric] ?? 999;
}

function hasRobustOutlier(values: number[]): boolean {
  if (values.length < 5) return false;
  const median = robustMedian(values);
  const absDeviations = values.map((value) => Math.abs(value - median));
  const mad = robustMedian(absDeviations);
  if (mad <= 1e-9) return false;
  const scale = 1.4826 * mad;
  return values.some((value) => Math.abs(value - median) / scale > 4.5);
}

function robustMedian(values: number[]): number {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((left, right) => left - right);
  const mid = Math.floor(sorted.length / 2);
  if (sorted.length % 2 === 1) return sorted[mid];
  return (sorted[mid - 1] + sorted[mid]) / 2;
}
