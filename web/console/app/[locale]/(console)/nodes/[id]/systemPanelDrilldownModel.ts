import type { Asset } from "../../../../console/models";
import type { AnalyzedSeries } from "./nodeMetricsModel";
import {
parseFloatValue,
parseIntValue,
readMeta,
type MetricSnapshot,
type SignalTone
} from "./systemPanelMetrics";
import type { SystemDrilldownView } from "./systemPanelTypes";

export type DetailRow = {
  key: string;
  label: string;
  value: string;
};

export type DetailSummaryStat = {
  key: string;
  label: string;
  value: string;
  hint: string;
  tone: SignalTone;
};

export type DetailSection = {
  key: string;
  title: string;
  description: string;
  rows: DetailRow[];
};

export type DetailAction = {
  key: string;
  label: string;
  panel: string;
  variant?: "primary" | "secondary" | "ghost";
};

export type DrilldownContent = {
  title: string;
  subtitle: string;
  heroLabel: string;
  heroValue: string;
  heroHint: string;
  statusLabel: string;
  statusTone: SignalTone;
  summaryStats: DetailSummaryStat[];
  sections: DetailSection[];
  actions: DetailAction[];
  tips: string[];
};

export const HISTORY_METRICS: Record<SystemDrilldownView, string[]> = {
  cpu: ["cpu_used_percent", "temperature_celsius"],
  memory: ["memory_used_percent"],
  storage: ["disk_used_percent"],
  network: ["network_rx_bytes_per_sec", "network_tx_bytes_per_sec"],
};

export function formatLoad(value: number | null): string {
  if (value == null) {
    return "n/a";
  }
  return value.toFixed(2);
}

export function formatRatio(value: number | null): string {
  if (value == null || !Number.isFinite(value)) {
    return "n/a";
  }
  return `${value.toFixed(2)}x`;
}

export function formatFrequency(raw: string): string {
  const value = parseFloatValue(raw);
  if (value == null || value <= 0) {
    return "n/a";
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1)} GHz`;
  }
  return `${value.toFixed(0)} MHz`;
}

export function usageTone(
  value: number | undefined,
  warnThreshold: number,
  badThreshold: number,
): SignalTone {
  if (value == null || !Number.isFinite(value)) {
    return "neutral";
  }
  if (value >= badThreshold) {
    return "bad";
  }
  if (value >= warnThreshold) {
    return "warn";
  }
  return "ok";
}

export function backupFreshness(asset: Asset): { value: string; tone: SignalTone } {
  const backupState = readMeta(asset, "backup_state");
  const backupDays = parseFloatValue(readMeta(asset, "days_since_backup"));

  if (backupState === "none") {
    return { value: "No backups", tone: "bad" };
  }
  if (backupDays != null) {
    if (backupDays < 1) {
      const hours = Math.round(backupDays * 24);
      return { value: hours <= 0 ? "Backup today" : `${hours}h since backup`, tone: "ok" };
    }
    const rounded = Math.round(backupDays);
    if (backupDays >= 14) {
      return { value: `${rounded} days since backup`, tone: "bad" };
    }
    if (backupDays >= 7) {
      return { value: `${rounded} days since backup`, tone: "warn" };
    }
    return { value: `${rounded} days since backup`, tone: "ok" };
  }
  if (backupState !== "") {
    return { value: backupState, tone: "neutral" };
  }
  return { value: "n/a", tone: "neutral" };
}

export function cpuStatusLabel(tone: SignalTone): string {
  switch (tone) {
    case "ok":
      return "Headroom available";
    case "warn":
      return "CPU pressure rising";
    case "bad":
      return "CPU saturated";
    default:
      return "Waiting for telemetry";
  }
}

export function memoryStatusLabel(tone: SignalTone): string {
  switch (tone) {
    case "ok":
      return "Memory headroom stable";
    case "warn":
      return "Memory pressure building";
    case "bad":
      return "Memory pressure high";
    default:
      return "Waiting for telemetry";
  }
}

export function storageStatusLabel(tone: SignalTone): string {
  switch (tone) {
    case "ok":
      return "Storage headroom healthy";
    case "warn":
      return "Capacity tightening";
    case "bad":
      return "Disk nearly full";
    default:
      return "Waiting for telemetry";
  }
}

export function networkStatus(
  asset: Asset,
  metrics: MetricSnapshot,
): { label: string; tone: SignalTone } {
  const interfaceCount = parseIntValue(readMeta(asset, "network_interface_count"));
  const primaryIP = readMeta(asset, "ip") || readMeta(asset, "ip_address");
  const tailscaleState = readMeta(asset, "tailscale_backend_state").toLowerCase();

  if (interfaceCount != null && interfaceCount <= 0) {
    return { label: "No interfaces reported", tone: "bad" };
  }
  if (primaryIP === "" && interfaceCount != null && interfaceCount > 0) {
    return { label: "Addressing needs review", tone: "warn" };
  }
  if (tailscaleState !== "" && tailscaleState !== "running" && tailscaleState !== "connected") {
    return { label: "Overlay network not ready", tone: "warn" };
  }
  if (metrics.network_rx_bytes_per_sec == null && metrics.network_tx_bytes_per_sec == null) {
    return { label: "Traffic snapshot unavailable", tone: "neutral" };
  }
  return { label: "Network path looks healthy", tone: "ok" };
}

export function relevantHistorySeries(
  analyzedSeries: AnalyzedSeries[],
  view: SystemDrilldownView,
): AnalyzedSeries[] {
  const metrics = HISTORY_METRICS[view];
  return metrics
    .map((metric) => analyzedSeries.find((series) => series.metric === metric) ?? null)
    .filter((series): series is AnalyzedSeries => series !== null);
}
