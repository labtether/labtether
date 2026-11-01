import type { Asset } from "../../../../console/models";
import {
  MISSING,
  backupFreshness,
  formatFrequency,
  formatLoad,
  formatRatio,
  networkStatus,
  usageStatusWord,
  usageTone,
  type DetailRow,
  type DrilldownContent,
} from "./systemPanelDrilldownModel";
import {
  formatBytes,
  formatPercent,
  formatRate,
  parseFloatValue,
  parseIntValue,
  readMeta,
  type MetricSnapshot,
  type SignalTone,
} from "./systemPanelMetrics";
import type { SystemDrilldownView } from "./systemPanelTypes";
export { relevantHistorySeries } from "./systemPanelDrilldownModel";
export type { DrilldownContent } from "./systemPanelDrilldownModel";

const orDash = (value: string) => value || MISSING;
const pct = (value: number | undefined) => {
  const text = formatPercent(value);
  return text === "n/a" ? MISSING : text;
};
const rate = (value: number | undefined) => {
  const text = formatRate(value);
  return text === "n/a" ? MISSING : text;
};
const metaRate = (asset: Asset, key: string) => rate(parseFloatValue(readMeta(asset, key)) ?? undefined);

/** Load per thread: amber from 0.7, red from 1 (more runnable work than threads). */
function loadTone(loadPerThread: number | null): SignalTone {
  if (loadPerThread == null) return "neutral";
  if (loadPerThread >= 1) return "bad";
  return loadPerThread >= 0.7 ? "warn" : "ok";
}

function cpuContent(asset: Asset, metrics: MetricSnapshot): DrilldownContent {
  const load1 = parseFloatValue(readMeta(asset, "load_1m"));
  const logicalThreads = parseIntValue(readMeta(asset, "cpu_threads_logical"));
  const loadPerThread = load1 != null && logicalThreads != null && logicalThreads > 0 ? load1 / logicalThreads : null;
  const minMHz = formatFrequency(readMeta(asset, "cpu_min_mhz"));
  const maxMHz = formatFrequency(readMeta(asset, "cpu_max_mhz"));
  const frequency = minMHz && maxMHz ? `${minMHz} – ${maxMHz}` : minMHz || maxMHz;
  const tone = usageTone(metrics.cpu_used_percent, 65, 85);
  const rows: DetailRow[] = [
    { key: "usage", value: pct(metrics.cpu_used_percent), tone },
    { key: "load1m", value: formatLoad(parseFloatValue(readMeta(asset, "load_1m"))) },
    { key: "load5m", value: formatLoad(parseFloatValue(readMeta(asset, "load_5m"))) },
    { key: "load15m", value: formatLoad(parseFloatValue(readMeta(asset, "load_15m"))) },
    { key: "loadPerThread", value: formatRatio(loadPerThread), tone: loadTone(loadPerThread) },
    { key: "temperature", value: orDash(readMeta(asset, "temperature_celsius") || readMeta(asset, "temp_celsius")) },
  ];
  return {
    view: "cpu",
    heroValue: pct(metrics.cpu_used_percent),
    heroHint: readMeta(asset, "cpu_model"),
    statusTone: tone,
    statusWord: usageStatusWord(tone),
    sections: [
      { key: "load", rows },
      {
        key: "topology",
        rows: [
          { key: "model", value: orDash(readMeta(asset, "cpu_model")) },
          { key: "vendor", value: orDash(readMeta(asset, "cpu_vendor")) },
          { key: "architecture", value: orDash(readMeta(asset, "cpu_architecture")) },
          { key: "sockets", value: orDash(readMeta(asset, "cpu_sockets")) },
          { key: "physicalCores", value: orDash(readMeta(asset, "cpu_cores_physical")) },
          { key: "logicalThreads", value: orDash(readMeta(asset, "cpu_threads_logical")) },
          { key: "frequency", value: orDash(frequency) },
        ],
      },
    ],
    actions: [
      { key: "metrics", panel: "monitoring" },
      { key: "processes", panel: "processes" },
      { key: "logs", panel: "logs" },
    ],
  };
}

function memoryContent(asset: Asset, metrics: MetricSnapshot): DrilldownContent {
  const totalRaw = readMeta(asset, "memory_total_bytes") || readMeta(asset, "physmem");
  const availableRaw = readMeta(asset, "memory_available_bytes");
  const total = parseFloatValue(totalRaw);
  const available = parseFloatValue(availableRaw);
  const used = total != null && available != null ? Math.max(total - available, 0) : null;
  const swapUsedRaw = readMeta(asset, "swap_used_bytes");
  const swapUsed = parseFloatValue(swapUsedRaw);
  const tone = usageTone(metrics.memory_used_percent, 75, 90);
  return {
    view: "memory",
    heroValue: pct(metrics.memory_used_percent),
    heroHint: formatBytes(totalRaw),
    statusTone: tone,
    statusWord: usageStatusWord(tone),
    sections: [
      {
        key: "capacity",
        rows: [
          { key: "installed", value: orDash(formatBytes(totalRaw)) },
          { key: "used", value: used != null ? orDash(formatBytes(String(used))) : MISSING },
          { key: "available", value: orDash(formatBytes(availableRaw)) },
          { key: "usage", value: pct(metrics.memory_used_percent), tone },
        ],
      },
      {
        key: "swap",
        rows: [
          { key: "swapTotal", value: orDash(formatBytes(readMeta(asset, "swap_total_bytes"))) },
          { key: "swapUsed", value: orDash(formatBytes(swapUsedRaw)), tone: swapUsed != null && swapUsed > 0 ? "warn" : "neutral" },
          { key: "os", value: orDash(readMeta(asset, "os_pretty_name")) },
          { key: "kernel", value: orDash(readMeta(asset, "kernel_release")) },
        ],
      },
    ],
    actions: [
      { key: "metrics", panel: "monitoring" },
      { key: "processes", panel: "processes" },
      { key: "logs", panel: "logs" },
    ],
  };
}

function storageContent(asset: Asset, metrics: MetricSnapshot): DrilldownContent {
  const rootCapacity = formatBytes(readMeta(asset, "disk_root_total_bytes")) || "n/a";
  const rootFreeRaw = readMeta(asset, "disk_root_available_bytes");
  const rootFree = rootFreeRaw === "0" ? "0 MiB" : formatBytes(rootFreeRaw) || "n/a";
  const backup = backupFreshness(asset);
  const tone = usageTone(metrics.disk_used_percent, 80, 92);
  const dash = (value: string) => (value === "n/a" ? MISSING : value);
  return {
    view: "storage",
    heroValue: pct(metrics.disk_used_percent),
    // Kept word for word: the storage tests read this line.
    heroHint: rootCapacity === "n/a" ? "" : `${rootFree} free of ${rootCapacity}`,
    statusTone: tone,
    statusWord: usageStatusWord(tone),
    sections: [
      {
        key: "rootFilesystem",
        rows: [
          { key: "capacity", value: dash(rootCapacity) },
          { key: "free", value: dash(rootFree), tone: tone === "warn" || tone === "bad" ? tone : "neutral" },
          { key: "usage", value: pct(metrics.disk_used_percent), tone },
          { key: "reportedUsage", value: orDash(readMeta(asset, "disk_percent")) },
        ],
      },
      {
        key: "ioBackup",
        rows: [
          { key: "read", value: metaRate(asset, "disk_read_bytes_per_sec") },
          { key: "write", value: metaRate(asset, "disk_write_bytes_per_sec") },
          { key: "backupAge", value: backup.value, tone: backup.tone },
          { key: "lastBackup", value: orDash(readMeta(asset, "last_backup_at")) },
        ],
      },
    ],
    actions: [
      { key: "metrics", panel: "monitoring" },
      { key: "actions", panel: "actions" },
      { key: "logs", panel: "logs" },
    ],
  };
}

function networkContent(asset: Asset, metrics: MetricSnapshot): DrilldownContent {
  const health = networkStatus(asset, metrics);
  const primaryIP = readMeta(asset, "ip") || readMeta(asset, "ip_address");
  return {
    view: "network",
    heroValue: `${rate(metrics.network_rx_bytes_per_sec)} / ${rate(metrics.network_tx_bytes_per_sec)}`,
    heroHint: primaryIP,
    statusTone: health.tone,
    statusWord: health.word,
    sections: [
      {
        key: "addressing",
        rows: [
          { key: "primaryIp", value: orDash(primaryIP), tone: health.word === "noAddress" ? "warn" : "neutral" },
          { key: "gateway", value: orDash(readMeta(asset, "network_default_gateway")) },
          { key: "dns", value: orDash(readMeta(asset, "network_dns_servers")) },
          { key: "interfaceCount", value: orDash(readMeta(asset, "network_interface_count")) },
        ],
      },
      {
        key: "tailscaleTraffic",
        rows: [
          { key: "receive", value: rate(metrics.network_rx_bytes_per_sec) },
          { key: "transmit", value: rate(metrics.network_tx_bytes_per_sec) },
          { key: "tailscaleState", value: orDash(readMeta(asset, "tailscale_backend_state")), tone: health.word === "tailscaleDown" ? "warn" : "neutral" },
          { key: "tailnet", value: orDash(readMeta(asset, "tailscale_tailnet")) },
          { key: "tailscaleIps", value: orDash(readMeta(asset, "tailscale_self_tailscale_ip")) },
          { key: "networkBackend", value: orDash(readMeta(asset, "network_backend")) },
          { key: "actionBackend", value: orDash(readMeta(asset, "network_action_backend")) },
        ],
      },
    ],
    actions: [
      { key: "interfaces", panel: "network" },
      { key: "metrics", panel: "monitoring" },
      { key: "logs", panel: "logs" },
    ],
  };
}

export function buildDrilldownContent(
  asset: Asset,
  metrics: MetricSnapshot,
  view: SystemDrilldownView,
): DrilldownContent {
  switch (view) {
    case "cpu":
      return cpuContent(asset, metrics);
    case "memory":
      return memoryContent(asset, metrics);
    case "storage":
      return storageContent(asset, metrics);
    case "network":
      return networkContent(asset, metrics);
  }
}
