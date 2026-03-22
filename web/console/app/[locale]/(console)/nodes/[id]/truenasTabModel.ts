export type TrueNASDiskHealth = {
  name: string;
  serial?: string;
  model?: string;
  type?: string;
  size_bytes?: number;
  temperature_celsius?: number;
  status: string;
  smart_enabled?: boolean;
  smart_health?: string;
  last_test_type?: string;
  last_test_status?: string;
  last_test_at?: string;
};

export type TrueNASSummary = {
  total: number;
  healthy: number;
  warning: number;
  critical: number;
  unknown: number;
};

export type TrueNASSmartResponse = {
  asset_id?: string;
  collector_id?: string;
  summary?: TrueNASSummary;
  disks?: TrueNASDiskHealth[];
  warnings?: string[];
  fetched_at?: string;
  error?: string;
};

export type TrueNASEvent = {
  id: string;
  level: string;
  message: string;
  timestamp: string;
  fields?: Record<string, string>;
};

export type TrueNASEventsResponse = {
  events?: TrueNASEvent[];
  fetched_at?: string;
  error?: string;
};

export type TrueNASFilesystemEntry = {
  name: string;
  path: string;
  type: string;
  size_bytes?: number;
  modified_at?: string;
  is_directory: boolean;
  is_symbolic?: boolean;
};

export type TrueNASFilesystemResponse = {
  path?: string;
  parent_path?: string;
  entries?: TrueNASFilesystemEntry[];
  error?: string;
};

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function asNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function asStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((entry): entry is string => typeof entry === "string");
}

export function normalizeTrueNASSmartResponse(value: unknown): TrueNASSmartResponse {
  const raw = asRecord(value) ?? {};
  const summary = asRecord(raw.summary);
  return {
    asset_id: asString(raw.asset_id),
    collector_id: asString(raw.collector_id),
    summary: summary
      ? {
          total: asNumber(summary.total) ?? 0,
          healthy: asNumber(summary.healthy) ?? 0,
          warning: asNumber(summary.warning) ?? 0,
          critical: asNumber(summary.critical) ?? 0,
          unknown: asNumber(summary.unknown) ?? 0,
        }
      : undefined,
    disks: Array.isArray(raw.disks)
      ? raw.disks.map((entry) => {
          const disk = asRecord(entry) ?? {};
          return {
            name: asString(disk.name) ?? "",
            serial: asString(disk.serial),
            model: asString(disk.model),
            type: asString(disk.type),
            size_bytes: asNumber(disk.size_bytes),
            temperature_celsius: asNumber(disk.temperature_celsius),
            status: asString(disk.status) ?? "",
            smart_enabled: typeof disk.smart_enabled === "boolean" ? disk.smart_enabled : undefined,
            smart_health: asString(disk.smart_health),
            last_test_type: asString(disk.last_test_type),
            last_test_status: asString(disk.last_test_status),
            last_test_at: asString(disk.last_test_at),
          };
        })
      : [],
    warnings: asStringArray(raw.warnings),
    fetched_at: asString(raw.fetched_at),
    error: asString(raw.error),
  };
}

export function normalizeTrueNASEventsResponse(value: unknown): TrueNASEventsResponse {
  const raw = asRecord(value) ?? {};
  return {
    events: Array.isArray(raw.events)
      ? raw.events.map((entry) => {
          const event = asRecord(entry) ?? {};
          return {
            id: asString(event.id) ?? "",
            level: asString(event.level) ?? "",
            message: asString(event.message) ?? "",
            timestamp: asString(event.timestamp) ?? "",
            fields: (() => {
              const fields = asRecord(event.fields);
              if (!fields) return undefined;
              const normalized: Record<string, string> = {};
              for (const [key, fieldValue] of Object.entries(fields)) {
                if (typeof fieldValue === "string") {
                  normalized[key] = fieldValue;
                }
              }
              return normalized;
            })(),
          };
        })
      : [],
    fetched_at: asString(raw.fetched_at),
    error: asString(raw.error),
  };
}

export function normalizeTrueNASFilesystemResponse(value: unknown): TrueNASFilesystemResponse {
  const raw = asRecord(value) ?? {};
  return {
    path: asString(raw.path),
    parent_path: asString(raw.parent_path),
    entries: Array.isArray(raw.entries)
      ? raw.entries.map((entry) => {
          const file = asRecord(entry) ?? {};
          return {
            name: asString(file.name) ?? "",
            path: asString(file.path) ?? "",
            type: asString(file.type) ?? "",
            size_bytes: asNumber(file.size_bytes),
            modified_at: asString(file.modified_at),
            is_directory: typeof file.is_directory === "boolean" ? file.is_directory : false,
            is_symbolic: typeof file.is_symbolic === "boolean" ? file.is_symbolic : undefined,
          };
        })
      : [],
    error: asString(raw.error),
  };
}

export function diskStatusBadge(status: string): "ok" | "pending" | "bad" {
  const normalized = status.toLowerCase();
  if (normalized === "critical") return "bad";
  if (normalized === "warning") return "pending";
  return "ok";
}

export function eventLevelBadge(level: string): "ok" | "pending" | "bad" {
  const normalized = level.toLowerCase();
  if (normalized.includes("error") || normalized.includes("crit")) return "bad";
  if (normalized.includes("warn")) return "pending";
  return "ok";
}

export function formatRelativeTime(raw: string): string {
  const timestamp = new Date(raw).getTime();
  if (!Number.isFinite(timestamp)) return "n/a";
  const diffMs = Date.now() - timestamp;
  if (diffMs < 60_000) return `${Math.max(1, Math.floor(diffMs / 1000))}s ago`;
  if (diffMs < 3_600_000) return `${Math.floor(diffMs / 60_000)}m ago`;
  if (diffMs < 86_400_000) return `${Math.floor(diffMs / 3_600_000)}h ago`;
  return `${Math.floor(diffMs / 86_400_000)}d ago`;
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "-";
  if (bytes < 1) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const index = Math.min(units.length - 1, Math.max(0, Math.floor(Math.log(bytes) / Math.log(1024))));
  const value = bytes / Math.pow(1024, index);
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}
