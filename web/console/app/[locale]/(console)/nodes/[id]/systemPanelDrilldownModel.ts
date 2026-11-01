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

/** A worded value: the component translates `phrase` (under devices.hardware.values) with `values`. */
export type DetailPhrase = {
  phrase: string;
  values?: Record<string, string | number>;
};

/** One key-value row. The label is translated by `key` (devices.hardware.rows); the value is data or a phrase. */
export type DetailRow = {
  key: string;
  value: string | DetailPhrase;
  /** warn or bad colours the value as a problem word; ok and neutral read as plain text. */
  tone?: SignalTone;
};

/** A titled block of rows. The title is translated by `key` (devices.hardware.sections). */
export type DetailSection = {
  key: string;
  rows: DetailRow[];
};

/** A button to another System view or tab: the panel it opens, labelled by `key` (devices.hardware.actions). */
export type DetailAction = {
  key: string;
  panel: string;
};

export type DrilldownContent = {
  view: SystemDrilldownView;
  heroValue: string;
  heroHint: string;
  statusTone: SignalTone;
  /** The status word key (devices.hardware.status.<view>.<word>); empty while nothing is wrong. */
  statusWord: string;
  sections: DetailSection[];
  actions: DetailAction[];
};

export const HISTORY_METRICS: Record<SystemDrilldownView, string[]> = {
  cpu: ["cpu_used_percent", "temperature_celsius"],
  memory: ["memory_used_percent"],
  storage: ["disk_used_percent"],
  network: ["network_rx_bytes_per_sec", "network_tx_bytes_per_sec"],
};

/** The dash shown for a value the device does not report. */
export const MISSING = "—";

export function formatLoad(value: number | null): string {
  if (value == null) {
    return MISSING;
  }
  return value.toFixed(2);
}

export function formatRatio(value: number | null): string {
  if (value == null || !Number.isFinite(value)) {
    return MISSING;
  }
  return `${value.toFixed(2)}×`;
}

export function formatFrequency(raw: string): string {
  const value = parseFloatValue(raw);
  if (value == null || value <= 0) {
    return "";
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

/** How old the last backup is, as a phrase and a tone: amber after 7 days, red after 14 or with none. */
export function backupFreshness(asset: Asset): { value: DetailPhrase | string; tone: SignalTone } {
  const backupState = readMeta(asset, "backup_state");
  const backupDays = parseFloatValue(readMeta(asset, "days_since_backup"));

  if (backupState === "none") {
    return { value: { phrase: "noBackups" }, tone: "bad" };
  }
  if (backupDays != null) {
    if (backupDays < 1) {
      const hours = Math.round(backupDays * 24);
      return { value: hours <= 0 ? { phrase: "backupToday" } : { phrase: "backupHours", values: { count: hours } }, tone: "ok" };
    }
    const rounded = Math.round(backupDays);
    const tone: SignalTone = backupDays >= 14 ? "bad" : backupDays >= 7 ? "warn" : "ok";
    return { value: { phrase: "backupDays", values: { count: rounded } }, tone };
  }
  if (backupState !== "") {
    return { value: backupState, tone: "neutral" };
  }
  return { value: MISSING, tone: "neutral" };
}

/** The status word for a usage view: empty while usage is fine, else "warn" or "bad" (worded per view). */
export function usageStatusWord(tone: SignalTone): string {
  return tone === "warn" || tone === "bad" ? tone : "";
}

/** What is wrong with the device's network, if anything LabTether can tell from what the agent reports. */
export function networkStatus(
  asset: Asset,
  metrics: MetricSnapshot,
): { word: string; tone: SignalTone } {
  const interfaceCount = parseIntValue(readMeta(asset, "network_interface_count"));
  const primaryIP = readMeta(asset, "ip") || readMeta(asset, "ip_address");
  const tailscaleState = readMeta(asset, "tailscale_backend_state").toLowerCase();

  if (interfaceCount != null && interfaceCount <= 0) {
    return { word: "noInterfaces", tone: "bad" };
  }
  if (primaryIP === "" && interfaceCount != null && interfaceCount > 0) {
    return { word: "noAddress", tone: "warn" };
  }
  if (tailscaleState !== "" && tailscaleState !== "running" && tailscaleState !== "connected") {
    return { word: "tailscaleDown", tone: "warn" };
  }
  if (metrics.network_rx_bytes_per_sec == null && metrics.network_tx_bytes_per_sec == null) {
    return { word: "", tone: "neutral" };
  }
  return { word: "", tone: "ok" };
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
