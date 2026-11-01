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
