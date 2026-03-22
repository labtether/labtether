import { assetFreshnessLabel } from "../../console/formatters";
import type { TelemetryOverviewAsset } from "../../console/models";

export type FleetStatus = "online" | "unresponsive" | "offline" | "unknown";

export function timeAgo(iso: string): string {
  const seconds = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (seconds < 60) return "just now";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

export function normalizeStatus(node: TelemetryOverviewAsset): FleetStatus {
  const raw = node.status.trim().toLowerCase();
  if (raw === "online" || raw === "up" || raw === "ok" || raw === "healthy") return "online";
  if (raw === "stale" || raw === "unresponsive" || raw === "warning") return "unresponsive";
  if (raw === "offline" || raw === "down" || raw === "critical" || raw === "error") return "offline";
  const freshness = assetFreshnessLabel(node.last_seen_at);
  if (freshness === "online" || freshness === "unresponsive" || freshness === "offline") return freshness;
  return "unknown";
}

export function fleetAvg(
  telemetry: TelemetryOverviewAsset[],
  getter: (m: TelemetryOverviewAsset["metrics"]) => number | undefined
): number | null {
  const values = telemetry.map((a) => getter(a.metrics)).filter((v): v is number => v != null);
  if (values.length === 0) return null;
  return values.reduce((sum, v) => sum + v, 0) / values.length;
}

export function fmtPct(value: number | null): string {
  if (value == null) return "--";
  return `${Math.round(value)}%`;
}

export function severityScore(node: TelemetryOverviewAsset, status: FleetStatus): number {
  let score = 0;
  if (status === "offline") score += 300;
  else if (status === "unresponsive") score += 180;
  else if (status === "unknown") score += 100;
  const cpu = node.metrics.cpu_used_percent;
  const mem = node.metrics.memory_used_percent;
  const disk = node.metrics.disk_used_percent;
  if (cpu != null && cpu >= 90) score += 80;
  else if (cpu != null && cpu >= 70) score += 20;
  if (mem != null && mem >= 95) score += 80;
  else if (mem != null && mem >= 80) score += 20;
  if (disk != null && disk >= 95) score += 80;
  else if (disk != null && disk >= 80) score += 20;
  return score;
}

export function topMetricValue(node: TelemetryOverviewAsset): number {
  const cpu = node.metrics.cpu_used_percent ?? 0;
  const mem = node.metrics.memory_used_percent ?? 0;
  const disk = node.metrics.disk_used_percent ?? 0;
  return Math.max(cpu, mem, disk);
}

export function topMetricLabel(node: TelemetryOverviewAsset): string {
  const cpu = node.metrics.cpu_used_percent;
  const mem = node.metrics.memory_used_percent;
  const disk = node.metrics.disk_used_percent;
  const max = Math.max(cpu ?? 0, mem ?? 0, disk ?? 0);
  if (max === 0 && cpu == null && mem == null && disk == null) return "--";
  if (cpu != null && cpu === max) return `CPU ${Math.round(cpu)}%`;
  if (mem != null && mem === max) return `Mem ${Math.round(mem)}%`;
  if (disk != null && disk === max) return `Disk ${Math.round(disk)}%`;
  return "--";
}
