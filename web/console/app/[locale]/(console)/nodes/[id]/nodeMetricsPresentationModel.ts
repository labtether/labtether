import { formatMetric } from "../../../../console/formatters";

export type QualityTone = "bad" | "warn" | "info";

/** The value at which a metric turns amber: 85% for percentages, 80 °C for temperatures; none for rates. */
export function hotThreshold(unit: string): number | null {
  if (unit === "percent") return 85;
  if (unit === "celsius") return 80;
  return null;
}

export function isHot(value: number | null | undefined, unit: string): boolean {
  const threshold = hotThreshold(unit);
  return threshold != null && value != null && Number.isFinite(value) && value >= threshold;
}

/** A metric value for a readout: "23%", "41 °C", "1.2 MB/s"; a dash without a value. */
export function formatReadout(value: number | null | undefined, unit: string): string {
  if (value == null || !Number.isFinite(value)) return "—";
  if (unit === "percent") return `${value < 10 && value > 0 ? value.toFixed(1) : Math.round(value)}%`;
  if (unit === "celsius") return `${Math.round(value)} °C`;
  return formatMetric(value, unit);
}

/** A chart axis label: whole numbers, short units. */
export function formatAxisValue(value: number, unit: string): string {
  if (!Number.isFinite(value)) return "";
  if (unit === "percent") return `${Math.round(value)}%`;
  if (unit === "celsius") return `${Math.round(value)} °C`;
  if (unit === "bytes_per_sec") {
    const units = ["B/s", "KB/s", "MB/s", "GB/s"];
    let working = Math.max(0, value);
    let idx = 0;
    while (working >= 1024 && idx < units.length - 1) {
      working /= 1024;
      idx += 1;
    }
    return `${working >= 10 || working === 0 ? Math.round(working) : working.toFixed(1)} ${units[idx]}`;
  }
  if (unit === "mbit_per_sec") return `${value > 0 && value < 10 ? value.toFixed(1) : Math.round(value)} Mbit/s`;
  return Math.abs(value) >= 10 ? String(Math.round(value)) : value.toFixed(1);
}

export function formatSignedMetric(value: number, unit: string): string {
  if (!Number.isFinite(value) || value === 0) return "0";
  const sign = value > 0 ? "+" : "−";
  return `${sign}${formatReadout(Math.abs(value), unit)}`;
}

export function formatDurationSeconds(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`;
  return `${Math.round(seconds / 86400)}d`;
}

/** "10:00–11:00" on one day, "1 Nov 10:00 – 2 Nov 10:00" across days; empty when the range is unknown. */
export function formatRange(fromISO: string, toISO: string): string {
  const from = new Date(fromISO);
  const to = new Date(toISO);
  if (!Number.isFinite(from.getTime()) || !Number.isFinite(to.getTime())) {
    return "";
  }
  const time = (d: Date) => d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  if (from.toDateString() === to.toDateString()) {
    return `${time(from)}–${time(to)}`;
  }
  const day = (d: Date) => d.toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  return `${day(from)} – ${day(to)}`;
}

export function formatTs(ts: number): string {
  return new Date(ts * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
}
