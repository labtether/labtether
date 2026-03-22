export function assetFreshnessLabel(lastSeenISO: string): string {
  const diffMs = Date.now() - new Date(lastSeenISO).getTime();
  if (!Number.isFinite(diffMs) || diffMs < 0) {
    return "unknown";
  }
  if (diffMs < 65_000) {
    return "online";
  }
  if (diffMs < 300_000) {
    return "unresponsive";
  }
  return "offline";
}

export function formatAge(lastSeenISO: string): string {
  const diffMs = Date.now() - new Date(lastSeenISO).getTime();
  if (!Number.isFinite(diffMs) || diffMs < 0) {
    return "n/a";
  }

  const seconds = Math.floor(diffMs / 1000);
  if (seconds < 60) {
    return `${seconds}s ago`;
  }

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    return `${minutes}m ago`;
  }

  const hours = Math.floor(minutes / 60);
  return `${hours}h ago`;
}

export function formatTimestamp(iso: string): string {
  const value = new Date(iso);
  if (Number.isNaN(value.getTime())) {
    return "n/a";
  }
  return value.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function formatMetric(value?: number, unit?: string): string {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "--";
  }
  switch (unit) {
    case "percent":
      return `${value.toFixed(1)}%`;
    case "celsius":
      return `${value.toFixed(1)} C`;
    case "bytes_per_sec":
      return `${formatBytesPerSecond(value)}`;
    default:
      return value.toFixed(2);
  }
}

export function formatBytes(value: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let working = value;
  let idx = 0;
  while (working >= 1024 && idx < units.length - 1) {
    working /= 1024;
    idx += 1;
  }
  const precision = working >= 100 ? 0 : working >= 10 ? 1 : 2;
  return `${working.toFixed(precision)} ${units[idx]}`;
}

export function formatBytesPerSecond(value: number): string {
  const units = ["B/s", "KB/s", "MB/s", "GB/s"];
  let working = value;
  let idx = 0;
  while (working >= 1024 && idx < units.length - 1) {
    working /= 1024;
    idx += 1;
  }
  return `${working.toFixed(1)} ${units[idx]}`;
}

const KNOWN_LABELS: Readonly<Record<string, string>> = {
  physmem: "Physical Memory",
  cpu_cores: "CPU Cores",
  cpu_threads: "CPU Threads",
  maxcpu: "Max CPUs",
  maxmem: "Max Memory",
  maxdisk: "Max Disk",
  netin: "Network In",
  netout: "Network Out",
  diskread: "Disk Read",
  diskwrite: "Disk Write",
  vmid: "VM ID",
  hastate: "HA State",
  uptime: "Uptime",
  cpus: "CPUs",
  pveversion: "PVE Version",
  kversion: "Kernel Version",
  rootfs: "Root FS",
  maxswap: "Max Swap",
  balloon: "Memory Balloon",
  // Docker metadata
  container_id: "Container ID",
  image: "Image",
  state: "State",
  status: "Status",
  ports: "Ports",
  stack: "Stack",
  engine_version: "Engine Version",
  engine_os: "Engine OS",
  engine_arch: "Architecture",
  // Home Assistant metadata
  entity_id: "Entity ID",
  friendly_name: "Friendly Name",
  original_name: "Original Name",
  original_icon: "Original Icon",
  unit_of_measurement: "Unit",
  device_class: "Device Class",
  state_class: "State Class",
  entity_category: "Entity Category",
  last_changed: "Last Changed",
  last_updated: "Last Updated",
  supported_features: "Supported Features",
  assumed_state: "Assumed State",
  area_id: "Area",
  icon: "Icon",
  collector_id: "Collector ID",
  collector_base_url: "Base URL",
  base_url: "Base URL",
  connector_type: "Connector Type",
  discovered: "Discovered Entities",
};

const BYTE_KEYS: ReadonlySet<string> = new Set([
  "physmem",
  "maxmem",
  "maxdisk",
  "maxswap",
  "balloon",
  "netin",
  "netout",
  "diskread",
  "diskwrite",
  "rootfs",
]);

export function formatMetadataLabel(key: string): string {
  if (key in KNOWN_LABELS) {
    return KNOWN_LABELS[key];
  }
  return key
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (match) => match.toUpperCase());
}

export function formatMetadataValue(key: string, value: string): string {
  const trimmed = value.trim();
  if (trimmed === "") {
    return trimmed;
  }

  const numeric = Number(trimmed);
  const isNumeric = Number.isFinite(numeric);

  // Existing suffix-based rules — kept unchanged
  if (key.endsWith("_bytes_per_sec") && isNumeric) {
    return formatBytesPerSecond(numeric);
  }
  if (key.endsWith("_bytes") && isNumeric) {
    return formatBytes(numeric);
  }
  if (key.endsWith("_percent") && isNumeric) {
    return `${numeric.toFixed(1)}%`;
  }
  if (key.endsWith("_celsius") && isNumeric) {
    return `${numeric.toFixed(1)} C`;
  }
  if (key.endsWith("_mhz") && isNumeric) {
    return `${numeric.toFixed(0)} MHz`;
  }

  // Key-name-based byte values (no _bytes suffix)
  if (BYTE_KEYS.has(key) && isNumeric && numeric > 0) {
    return formatBytes(numeric);
  }

  // Booleans
  const lower = trimmed.toLowerCase();
  if (lower === "true") {
    return "Yes";
  }
  if (lower === "false") {
    return "No";
  }

  // Uptime (seconds as a numeric value)
  if (key === "uptime" && isNumeric && numeric >= 0) {
    const totalSeconds = Math.floor(numeric);
    const days = Math.floor(totalSeconds / 86400);
    const hours = Math.floor((totalSeconds % 86400) / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    if (days > 0) {
      return `${days}d ${hours}h`;
    }
    if (hours > 0) {
      return `${hours}h ${minutes}m`;
    }
    return `${minutes}m`;
  }

  // ISO timestamps
  if (/^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}/.test(trimmed)) {
    const date = new Date(trimmed);
    if (Number.isFinite(date.getTime())) {
      return date.toLocaleString([], {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
    }
  }

  return trimmed;
}

export function settingValue(
  settings: Array<{ key: string; effective_value?: string }>,
  key: string,
  fallback: string
): string {
  const entry = settings.find((item) => item.key === key);
  return entry?.effective_value?.trim() ? entry.effective_value.trim() : fallback;
}

export function parseIntSetting(value: string, fallback: number): number {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallback;
  }
  return parsed;
}

export function parseBoolSetting(value: string, fallback: boolean): boolean {
  const lowered = value.trim().toLowerCase();
  if (lowered === "true") {
    return true;
  }
  if (lowered === "false") {
    return false;
  }
  return fallback;
}

export function parseWindowSetting<T extends string>(value: string, allowed: readonly T[], fallback: T): T {
  if ((allowed as readonly string[]).includes(value)) {
    return value as T;
  }
  return fallback;
}

export function sourceClassName(source: string): string {
  switch (source) {
    case "ui":
      return "ok";
    case "docker":
      return "pending";
    default:
      return "";
  }
}
