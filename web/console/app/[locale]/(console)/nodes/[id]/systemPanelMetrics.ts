import type { CSSProperties } from "react";
import type {
  Asset,
  NodeMetadataSectionName,
  TelemetryOverviewAsset,
} from "../../../../console/models";
import type { NetworkInterfaceInfo } from "../../../../hooks/useNetworkInterfaces";
import type { SystemDrilldownView } from "./systemPanelTypes";

export type MetricSnapshot = {
  cpu_used_percent?: number;
  memory_used_percent?: number;
  disk_used_percent?: number;
  network_rx_bytes_per_sec?: number;
  network_tx_bytes_per_sec?: number;
};

export type SignalTone = "ok" | "warn" | "bad" | "neutral";

export const CURATED_SECTIONS: NodeMetadataSectionName[] = [
  "System",
  "Hardware",
  "CPU",
  "Memory",
  "Storage",
  "Firmware",
  "Network",
];

export const DRILLDOWN_ORDER: SystemDrilldownView[] = ["cpu", "memory", "storage", "network"];

export function readMeta(asset: Asset, key: string): string {
  return asset.metadata?.[key]?.trim() ?? "";
}

export function parseFloatValue(raw: string): number | null {
  const value = Number(raw);
  if (!Number.isFinite(value)) {
    return null;
  }
  return value;
}

export function parseIntValue(raw: string): number | null {
  const value = parseFloatValue(raw);
  if (value == null) {
    return null;
  }
  return Math.trunc(value);
}

export function formatBytes(raw: string): string {
  const value = parseFloatValue(raw);
  if (value == null || value <= 0) {
    return "";
  }
  const gib = value / (1024 * 1024 * 1024);
  if (gib >= 100) {
    return `${gib.toFixed(0)} GiB`;
  }
  if (gib >= 1) {
    return `${gib.toFixed(1)} GiB`;
  }
  const mib = value / (1024 * 1024);
  return `${mib.toFixed(0)} MiB`;
}

export function formatRate(raw: number | undefined): string {
  if (raw == null || !Number.isFinite(raw) || raw < 0) {
    return "n/a";
  }
  if (raw >= 1024 * 1024) {
    return `${(raw / (1024 * 1024)).toFixed(1)} MB/s`;
  }
  if (raw >= 1024) {
    return `${(raw / 1024).toFixed(1)} KB/s`;
  }
  return `${Math.round(raw)} B/s`;
}

export function formatPercent(value: number | undefined): string {
  if (value == null || !Number.isFinite(value)) {
    return "n/a";
  }
  return `${value.toFixed(1)}%`;
}

export function formatInterfaceAddress(entry: NetworkInterfaceInfo): string {
  if (entry.ips.length > 0) {
    return entry.ips[0];
  }
  if (entry.mac) {
    return entry.mac;
  }
  return "No address reported";
}

export function interfaceTraffic(entry: NetworkInterfaceInfo): number {
  return Math.max(0, entry.rx_bytes) + Math.max(0, entry.tx_bytes);
}

function fallbackMetrics(asset: Asset): MetricSnapshot {
  const metrics: MetricSnapshot = {};
  const cpu = parseFloatValue(readMeta(asset, "cpu_used_percent") || readMeta(asset, "cpu_percent"));
  if (cpu != null) {
    metrics.cpu_used_percent = cpu;
  }
  const mem = parseFloatValue(readMeta(asset, "memory_used_percent") || readMeta(asset, "memory_percent"));
  if (mem != null) {
    metrics.memory_used_percent = mem;
  }
  const disk = parseFloatValue(readMeta(asset, "disk_used_percent") || readMeta(asset, "disk_percent"));
  if (disk != null) {
    metrics.disk_used_percent = disk;
  }
  const rx = parseFloatValue(readMeta(asset, "network_rx_bytes_per_sec"));
  if (rx != null) {
    metrics.network_rx_bytes_per_sec = rx;
  }
  const tx = parseFloatValue(readMeta(asset, "network_tx_bytes_per_sec"));
  if (tx != null) {
    metrics.network_tx_bytes_per_sec = tx;
  }
  return metrics;
}

export function gatherMetrics(
  asset: Asset,
  telemetry: TelemetryOverviewAsset | null,
): MetricSnapshot {
  const fallback = fallbackMetrics(asset);
  return {
    cpu_used_percent: telemetry?.metrics?.cpu_used_percent ?? fallback.cpu_used_percent,
    memory_used_percent: telemetry?.metrics?.memory_used_percent ?? fallback.memory_used_percent,
    disk_used_percent: telemetry?.metrics?.disk_used_percent ?? fallback.disk_used_percent,
    network_rx_bytes_per_sec: telemetry?.metrics?.network_rx_bytes_per_sec ?? fallback.network_rx_bytes_per_sec,
    network_tx_bytes_per_sec: telemetry?.metrics?.network_tx_bytes_per_sec ?? fallback.network_tx_bytes_per_sec,
  };
}

export function toneColor(tone: SignalTone): string {
  switch (tone) {
    case "ok":
      return "var(--ok)";
    case "warn":
      return "var(--warn)";
    case "bad":
      return "var(--bad)";
    default:
      return "var(--muted)";
  }
}

function toneBackground(tone: SignalTone): string {
  switch (tone) {
    case "ok":
      return "var(--ok-glow)";
    case "warn":
      return "var(--warn-glow)";
    case "bad":
      return "var(--bad-glow)";
    default:
      return "var(--surface)";
  }
}

export function toneStyle(tone: SignalTone): CSSProperties {
  return {
    borderColor: tone === "neutral" ? "var(--line)" : toneColor(tone),
    backgroundColor: toneBackground(tone),
    color: toneColor(tone),
  };
}

export function drilldownLabel(view: SystemDrilldownView): string {
  switch (view) {
    case "cpu":
      return "CPU";
    case "memory":
      return "Memory";
    case "storage":
      return "Storage";
    case "network":
      return "Network";
  }
}
