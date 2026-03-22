import { formatMetric } from "../../../../console/formatters";
import type { AnalyzedSeries } from "./nodeMetricsAnalysisModel";

export type QualityTone = "bad" | "warn" | "info";

export type ThresholdBand = {
  key: string;
  low: number;
  high: number;
  color: string;
  opacity: number;
};

export function metricValueTone(value: number | null, unit: string): string {
  if (value == null) return "text-[var(--muted)]";
  if (unit === "percent") {
    if (value >= 90) return "text-[var(--bad)]";
    if (value >= 70) return "text-[var(--warn)]";
    return "text-[var(--ok)]";
  }
  if (unit === "celsius") {
    if (value >= 80) return "text-[var(--bad)]";
    if (value >= 70) return "text-[var(--warn)]";
    return "text-[var(--ok)]";
  }
  return "text-[var(--text)]";
}

export function dotToneClass(value: number | null, unit: string): string {
  const tone = metricValueTone(value, unit);
  if (tone === "text-[var(--bad)]") return "bg-[var(--bad)]";
  if (tone === "text-[var(--warn)]") return "bg-[var(--warn)]";
  if (tone === "text-[var(--ok)]") return "bg-[var(--ok)]";
  return "bg-[var(--muted)]";
}

export function flagToneClass(tone: QualityTone): string {
  if (tone === "bad") return "border-[var(--bad)]/50 text-[var(--bad)] bg-[var(--bad-glow)]";
  if (tone === "warn") return "border-[var(--warn)]/50 text-[var(--warn)] bg-[var(--warn-glow)]";
  return "border-[var(--line)] text-[var(--muted)] bg-[var(--surface)]";
}

export function formatSignedMetric(value: number, unit: string): string {
  if (!Number.isFinite(value) || value === 0) return "0";
  const sign = value > 0 ? "+" : "-";
  return `${sign}${formatMetric(Math.abs(value), unit)}`;
}

export function formatDurationSeconds(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`;
  return `${Math.round(seconds / 86400)}d`;
}

export function formatRange(fromISO: string, toISO: string): string {
  const from = new Date(fromISO);
  const to = new Date(toISO);
  if (!Number.isFinite(from.getTime()) || !Number.isFinite(to.getTime())) {
    return "Time range unavailable";
  }
  const sameDay = from.toDateString() === to.toDateString();
  if (sameDay) {
    return `${from.toLocaleDateString()} ${from.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} - ${to.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
  }
  return `${from.toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })} - ${to.toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}`;
}

export function formatTs(ts: number): string {
  return new Date(ts * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function isCriticalThreshold(unit: string, value: number): boolean {
  if (unit === "percent") return value >= 90;
  if (unit === "celsius") return value >= 80;
  return false;
}

export function thresholdBands(series: AnalyzedSeries): ThresholdBand[] {
  if (series.unit === "percent") {
    const warningHigh = Math.min(series.yMax, 90);
    const warningBand = warningHigh > 70
      ? [{ key: "percent-warn", low: 70, high: warningHigh, color: "#f59e0b", opacity: 0.08 }]
      : [];
    const criticalBand = series.yMax > 90
      ? [{ key: "percent-critical", low: 90, high: series.yMax, color: "#ef4444", opacity: 0.08 }]
      : [];
    return [...warningBand, ...criticalBand];
  }
  if (series.unit === "celsius") {
    const warningHigh = Math.min(series.yMax, 80);
    const warningBand = warningHigh > 70
      ? [{ key: "temp-warn", low: 70, high: warningHigh, color: "#f59e0b", opacity: 0.08 }]
      : [];
    const criticalBand = series.yMax > 80
      ? [{ key: "temp-critical", low: 80, high: series.yMax, color: "#ef4444", opacity: 0.08 }]
      : [];
    return [...warningBand, ...criticalBand];
  }
  return [];
}
