import type { Asset } from "../../../../console/models";
import type { AnalyzedSeries } from "./nodeMetricsModel";
import type { SystemDrilldownView } from "./systemPanelTypes";
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

type DetailRow = {
  key: string;
  label: string;
  value: string;
};

type DetailSummaryStat = {
  key: string;
  label: string;
  value: string;
  hint: string;
  tone: SignalTone;
};

type DetailSection = {
  key: string;
  title: string;
  description: string;
  rows: DetailRow[];
};

type DetailAction = {
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

const HISTORY_METRICS: Record<SystemDrilldownView, string[]> = {
  cpu: ["cpu_used_percent", "temperature_celsius"],
  memory: ["memory_used_percent"],
  storage: ["disk_used_percent"],
  network: ["network_rx_bytes_per_sec", "network_tx_bytes_per_sec"],
};

function formatLoad(value: number | null): string {
  if (value == null) {
    return "n/a";
  }
  return value.toFixed(2);
}

function formatRatio(value: number | null): string {
  if (value == null || !Number.isFinite(value)) {
    return "n/a";
  }
  return `${value.toFixed(2)}x`;
}

function formatFrequency(raw: string): string {
  const value = parseFloatValue(raw);
  if (value == null || value <= 0) {
    return "n/a";
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1)} GHz`;
  }
  return `${value.toFixed(0)} MHz`;
}

function usageTone(
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

function backupFreshness(asset: Asset): { value: string; tone: SignalTone } {
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

function cpuStatusLabel(tone: SignalTone): string {
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

function memoryStatusLabel(tone: SignalTone): string {
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

function storageStatusLabel(tone: SignalTone): string {
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

function networkStatus(
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

export function buildDrilldownContent(
  asset: Asset,
  metrics: MetricSnapshot,
  view: SystemDrilldownView,
): DrilldownContent {
  const memoryTotalRaw = readMeta(asset, "memory_total_bytes") || readMeta(asset, "physmem");
  const memoryAvailableRaw = readMeta(asset, "memory_available_bytes");
  const memoryTotal = parseFloatValue(memoryTotalRaw);
  const memoryAvailable = parseFloatValue(memoryAvailableRaw);
  const memoryUsed = memoryTotal != null && memoryAvailable != null
    ? Math.max(memoryTotal - memoryAvailable, 0)
    : null;
  const load1 = parseFloatValue(readMeta(asset, "load_1m"));
  const load5 = parseFloatValue(readMeta(asset, "load_5m"));
  const load15 = parseFloatValue(readMeta(asset, "load_15m"));
  const logicalThreads = parseIntValue(readMeta(asset, "cpu_threads_logical"));
  const sockets = readMeta(asset, "cpu_sockets") || "n/a";
  const physicalCores = readMeta(asset, "cpu_cores_physical") || "n/a";
  const logicalThreadsLabel = readMeta(asset, "cpu_threads_logical") || "n/a";
  const loadPerThread = load1 != null && logicalThreads != null && logicalThreads > 0
    ? load1 / logicalThreads
    : null;
  const rootCapacity = formatBytes(readMeta(asset, "disk_root_total_bytes")) || "n/a";
  const rootFree = formatBytes(readMeta(asset, "disk_root_available_bytes")) || "n/a";
  const backup = backupFreshness(asset);
  const networkHealth = networkStatus(asset, metrics);
  const primaryIP = readMeta(asset, "ip") || readMeta(asset, "ip_address") || "n/a";
  const tailscaleIPs = readMeta(asset, "tailscale_self_tailscale_ip") || "n/a";

  switch (view) {
    case "cpu": {
      const tone = usageTone(metrics.cpu_used_percent, 65, 85);
      return {
        title: "CPU Deep Detail",
        subtitle: "Topology, frequency envelope, live saturation, and short-window trend context.",
        heroLabel: "Current CPU utilization",
        heroValue: formatPercent(metrics.cpu_used_percent),
        heroHint: readMeta(asset, "cpu_model") || "Live CPU snapshot",
        statusLabel: cpuStatusLabel(tone),
        statusTone: tone,
        summaryStats: [
          {
            key: "cpu-topology",
            label: "Topology",
            value: `${physicalCores}C / ${logicalThreadsLabel}T`,
            hint: `${sockets} socket(s)`,
            tone: "neutral",
          },
          {
            key: "cpu-load",
            label: "Load / Thread",
            value: formatRatio(loadPerThread),
            hint: load1 != null && logicalThreads != null
              ? `${load1.toFixed(2)} over ${logicalThreads} threads`
              : "Needs load average and thread count",
            tone: loadPerThread == null
              ? "neutral"
              : loadPerThread >= 1
                ? "bad"
                : loadPerThread >= 0.7
                  ? "warn"
                  : "ok",
          },
          {
            key: "cpu-frequency",
            label: "Frequency Envelope",
            value: `${formatFrequency(readMeta(asset, "cpu_min_mhz"))} - ${formatFrequency(readMeta(asset, "cpu_max_mhz"))}`,
            hint: readMeta(asset, "cpu_vendor") || "Vendor not reported",
            tone: "neutral",
          },
        ],
        sections: [
          {
            key: "cpu-pressure",
            title: "Live Pressure",
            description: "Use these signals to tell a steady bottleneck from a short burst.",
            rows: [
              { key: "cpu-usage", label: "Current Usage", value: formatPercent(metrics.cpu_used_percent) },
              { key: "cpu-load-1m", label: "Load (1m)", value: formatLoad(load1) },
              { key: "cpu-load-5m", label: "Load (5m)", value: formatLoad(load5) },
              { key: "cpu-load-15m", label: "Load (15m)", value: formatLoad(load15) },
              { key: "cpu-load-thread", label: "Load / Thread", value: formatRatio(loadPerThread) },
              { key: "cpu-temp", label: "Temperature", value: readMeta(asset, "temperature_celsius") || readMeta(asset, "temp_celsius") || "n/a" },
            ],
          },
          {
            key: "cpu-topology-detail",
            title: "Compute Topology",
            description: "Hardware shape and placement help explain why load behaves the way it does.",
            rows: [
              { key: "cpu-model", label: "Model", value: readMeta(asset, "cpu_model") || "n/a" },
              { key: "cpu-vendor", label: "Vendor", value: readMeta(asset, "cpu_vendor") || "n/a" },
              { key: "cpu-arch", label: "Architecture", value: readMeta(asset, "cpu_architecture") || "n/a" },
              { key: "cpu-sockets", label: "Sockets", value: sockets },
              { key: "cpu-physical", label: "Physical Cores", value: physicalCores },
              { key: "cpu-logical", label: "Logical Threads", value: logicalThreadsLabel },
            ],
          },
        ],
        actions: [
          { key: "cpu-metrics", label: "Open Metrics", panel: "monitoring", variant: "primary" },
          { key: "cpu-logs", label: "Open Logs", panel: "logs", variant: "ghost" },
          { key: "cpu-processes", label: "Open Processes", panel: "processes", variant: "ghost" },
        ],
        tips: [
          "Use Metrics next if the spike looks real and you need to confirm whether it is sustained over time.",
          "Cross-check Logs when CPU rises suddenly without a matching workload change.",
          "Compare load-per-thread against the topology card to spot scheduling pressure on smaller hosts.",
        ],
      };
    }
    case "memory": {
      const tone = usageTone(metrics.memory_used_percent, 75, 90);
      return {
        title: "Memory Deep Detail",
        subtitle: "Capacity, swap posture, immediate pressure, and short-window trend context.",
        heroLabel: "Current memory utilization",
        heroValue: formatPercent(metrics.memory_used_percent),
        heroHint: formatBytes(memoryTotalRaw) || "Installed memory unavailable",
        statusLabel: memoryStatusLabel(tone),
        statusTone: tone,
        summaryStats: [
          {
            key: "memory-installed",
            label: "Installed Memory",
            value: formatBytes(memoryTotalRaw) || "n/a",
            hint: readMeta(asset, "computer_model") || "System memory footprint",
            tone: "neutral",
          },
          {
            key: "memory-available",
            label: "Available",
            value: formatBytes(memoryAvailableRaw) || "n/a",
            hint: "Immediately reclaimable memory",
            tone: tone === "bad" ? "warn" : "ok",
          },
          {
            key: "memory-swap",
            label: "Swap Used",
            value: formatBytes(readMeta(asset, "swap_used_bytes")) || "n/a",
            hint: formatBytes(readMeta(asset, "swap_total_bytes")) || "Swap total unavailable",
            tone: parseFloatValue(readMeta(asset, "swap_used_bytes")) != null ? "warn" : "neutral",
          },
        ],
        sections: [
          {
            key: "memory-capacity",
            title: "Capacity Snapshot",
            description: "Physical memory accounting before you dig into process-level usage.",
            rows: [
              { key: "memory-total", label: "Installed Memory", value: formatBytes(memoryTotalRaw) || "n/a" },
              { key: "memory-used", label: "Used Memory", value: memoryUsed != null ? formatBytes(String(memoryUsed)) : "n/a" },
              { key: "memory-available", label: "Available Memory", value: formatBytes(memoryAvailableRaw) || "n/a" },
              { key: "memory-usage", label: "Current Usage", value: formatPercent(metrics.memory_used_percent) },
            ],
          },
          {
            key: "memory-pressure",
            title: "Swap And Pressure",
            description: "Swap growth often confirms real memory pressure instead of transient cache use.",
            rows: [
              { key: "memory-swap-total", label: "Swap Total", value: formatBytes(readMeta(asset, "swap_total_bytes")) || "n/a" },
              { key: "memory-swap-used", label: "Swap Used", value: formatBytes(readMeta(asset, "swap_used_bytes")) || "n/a" },
              { key: "memory-os", label: "OS", value: readMeta(asset, "os_pretty_name") || "n/a" },
              { key: "memory-kernel", label: "Kernel Release", value: readMeta(asset, "kernel_release") || "n/a" },
            ],
          },
        ],
        actions: [
          { key: "memory-metrics", label: "Open Metrics", panel: "monitoring", variant: "primary" },
          { key: "memory-logs", label: "Open Logs", panel: "logs", variant: "ghost" },
          { key: "memory-processes", label: "Open Processes", panel: "processes", variant: "ghost" },
        ],
        tips: [
          "Use Metrics to see whether memory pressure is climbing steadily or just reflecting a recent cache warm-up.",
          "Swap use without recovery is usually a stronger signal than a single high memory snapshot.",
          "Pair this view with Logs when OOMs, restarts, or reclaim storms are suspected.",
        ],
      };
    }
    case "storage": {
      const tone = usageTone(metrics.disk_used_percent, 80, 92);
      return {
        title: "Storage Deep Detail",
        subtitle: "Filesystem capacity, throughput clues, backup posture, and short-window trend context.",
        heroLabel: "Root filesystem utilization",
        heroValue: formatPercent(metrics.disk_used_percent),
        heroHint: rootCapacity === "n/a" ? "Root capacity unavailable" : `${rootFree} free of ${rootCapacity}`,
        statusLabel: storageStatusLabel(tone),
        statusTone: tone,
        summaryStats: [
          {
            key: "storage-free",
            label: "Root Free",
            value: rootFree,
            hint: rootCapacity === "n/a" ? "Capacity unavailable" : `Capacity ${rootCapacity}`,
            tone: tone === "bad" ? "bad" : tone === "warn" ? "warn" : "ok",
          },
          {
            key: "storage-io",
            label: "Read / Write",
            value: `${formatRate(parseFloatValue(readMeta(asset, "disk_read_bytes_per_sec")) ?? undefined)} / ${formatRate(parseFloatValue(readMeta(asset, "disk_write_bytes_per_sec")) ?? undefined)}`,
            hint: "Current throughput snapshot",
            tone: "neutral",
          },
          {
            key: "storage-backup",
            label: "Backup Freshness",
            value: backup.value,
            hint: readMeta(asset, "last_backup_at") || "Last backup not reported",
            tone: backup.tone,
          },
        ],
        sections: [
          {
            key: "storage-capacity",
            title: "Filesystem Capacity",
            description: "Use this when free space feels tight or growth is hard to explain.",
            rows: [
              { key: "storage-total", label: "Root Capacity", value: rootCapacity },
              { key: "storage-free", label: "Root Free", value: rootFree },
              { key: "storage-used", label: "Root Usage", value: formatPercent(metrics.disk_used_percent) },
              { key: "storage-disk-percent", label: "Reported Disk Used", value: readMeta(asset, "disk_percent") || "n/a" },
            ],
          },
          {
            key: "storage-ops",
            title: "Operational Signals",
            description: "Storage health is not just space: I/O and backup recency matter for triage too.",
            rows: [
              { key: "storage-read", label: "Read Throughput", value: formatRate(parseFloatValue(readMeta(asset, "disk_read_bytes_per_sec")) ?? undefined) },
              { key: "storage-write", label: "Write Throughput", value: formatRate(parseFloatValue(readMeta(asset, "disk_write_bytes_per_sec")) ?? undefined) },
              { key: "storage-backup-freshness", label: "Backup Freshness", value: backup.value },
              { key: "storage-last-backup", label: "Last Backup", value: readMeta(asset, "last_backup_at") || "n/a" },
            ],
          },
        ],
        actions: [
          { key: "storage-metrics", label: "Open Metrics", panel: "monitoring", variant: "primary" },
          { key: "storage-logs", label: "Open Logs", panel: "logs", variant: "ghost" },
          { key: "storage-actions", label: "Open Actions", panel: "actions", variant: "ghost" },
        ],
        tips: [
          "Follow with Metrics when utilization is climbing and you need to see if this is a long-running trend.",
          "Use Logs for filesystem, backup, or I/O error context before taking cleanup action.",
          "Backup freshness is surfaced here so storage triage includes recoverability, not just free space.",
        ],
      };
    }
    case "network":
      return {
        title: "Network Deep Detail",
        subtitle: "Addressing, overlay status, live traffic context, and direct interface previews.",
        heroLabel: "Current traffic snapshot",
        heroValue: `${formatRate(metrics.network_rx_bytes_per_sec)} / ${formatRate(metrics.network_tx_bytes_per_sec)}`,
        heroHint: `${readMeta(asset, "network_interface_count") || "n/a"} interfaces reported`,
        statusLabel: networkHealth.label,
        statusTone: networkHealth.tone,
        summaryStats: [
          {
            key: "network-primary-ip",
            label: "Primary IP",
            value: primaryIP,
            hint: readMeta(asset, "network_default_gateway") || "Gateway unavailable",
            tone: primaryIP === "n/a" ? "warn" : "ok",
          },
          {
            key: "network-overlay",
            label: "Tailscale",
            value: readMeta(asset, "tailscale_backend_state") || "n/a",
            hint: readMeta(asset, "tailscale_tailnet") || "Tailnet unavailable",
            tone: networkHealth.tone,
          },
          {
            key: "network-interfaces",
            label: "Interfaces",
            value: readMeta(asset, "network_interface_count") || "n/a",
            hint: readMeta(asset, "network_backend") || "Backend unavailable",
            tone: "neutral",
          },
        ],
        sections: [
          {
            key: "network-addressing",
            title: "Addressing",
            description: "Confirm the basics first: interface count, IP identity, and how traffic exits the node.",
            rows: [
              { key: "network-primary-ip", label: "Primary IP", value: primaryIP },
              { key: "network-gateway", label: "Default Gateway", value: readMeta(asset, "network_default_gateway") || "n/a" },
              { key: "network-dns", label: "DNS Servers", value: readMeta(asset, "network_dns_servers") || "n/a" },
              { key: "network-interfaces", label: "Interface Count", value: readMeta(asset, "network_interface_count") || "n/a" },
            ],
          },
          {
            key: "network-overlay-traffic",
            title: "Overlay And Traffic",
            description: "Use overlay state and live throughput together when remote access feels flaky.",
            rows: [
              { key: "network-rx", label: "Receive Rate", value: formatRate(metrics.network_rx_bytes_per_sec) },
              { key: "network-tx", label: "Transmit Rate", value: formatRate(metrics.network_tx_bytes_per_sec) },
              { key: "network-tailscale-state", label: "Tailscale State", value: readMeta(asset, "tailscale_backend_state") || "n/a" },
              { key: "network-tailscale-ip", label: "Tailscale IPs", value: tailscaleIPs },
              { key: "network-backend", label: "Network Backend", value: readMeta(asset, "network_backend") || "n/a" },
              { key: "network-action-backend", label: "Action Backend", value: readMeta(asset, "network_action_backend") || "n/a" },
            ],
          },
        ],
        actions: [
          { key: "network-interfaces", label: "Open Interfaces", panel: "network", variant: "primary" },
          { key: "network-metrics", label: "Open Metrics", panel: "monitoring", variant: "ghost" },
          { key: "network-logs", label: "Open Logs", panel: "logs", variant: "ghost" },
        ],
        tips: [
          "Open Interfaces for per-link state, counters, and IP details when this snapshot suggests a bad path.",
          "Use Metrics next if traffic looks wrong but you need to confirm whether the issue is persistent.",
          "Overlay state is included here so remote-access and routing issues can be triaged without leaving System first.",
        ],
      };
  }
}
