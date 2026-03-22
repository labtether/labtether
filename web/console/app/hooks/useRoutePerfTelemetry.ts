"use client";

export type RoutePerfName = "dashboard" | "logs" | "devices" | "services" | "topology";
type RoutePerfStatus = "ok" | "error" | "aborted";

type RoutePerfPayload = {
  route: RoutePerfName;
  metric: string;
  durationMs: number;
  status?: RoutePerfStatus;
  sampleSize?: number;
  metadata?: Record<string, string | number | boolean | null | undefined>;
  throttleMs?: number;
};

const sendThrottleMs = 15_000;
const lastSentByMetric = new Map<string, number>();

export function routePerfNameFromPathname(pathname: string): RoutePerfName | null {
  const normalized = pathname.trim();
  if (normalized === "" || normalized === "/") {
    return "dashboard";
  }
  if (normalized === "/logs" || normalized.startsWith("/logs/")) {
    return "logs";
  }
  if (normalized === "/nodes" || normalized.startsWith("/nodes/")) {
    return "devices";
  }
  if (normalized === "/services" || normalized.startsWith("/services/")) {
    return "services";
  }
  if (normalized === "/topology" || normalized.startsWith("/topology/")) {
    return "topology";
  }
  return null;
}

export function currentRoutePerfName(): RoutePerfName | null {
  if (typeof window === "undefined") {
    return null;
  }
  return routePerfNameFromPathname(window.location.pathname);
}

export function reportRoutePerfMetric(payload: RoutePerfPayload): void {
  if (typeof window === "undefined") return;
  if (!isFinite(payload.durationMs) || payload.durationMs < 0) return;
  if (isAutomatedBrowser()) return;

  const metric = payload.metric.trim();
  if (!metric) return;

  const throttleKey = `${payload.route}:${metric}`;
  const now = Date.now();
  const lastSentAt = lastSentByMetric.get(throttleKey) ?? 0;
  const throttleWindowMs = normalizeThrottleWindow(payload.throttleMs);
  if (now - lastSentAt < throttleWindowMs) {
    return;
  }
  lastSentByMetric.set(throttleKey, now);

  const normalizedDuration = Math.round(payload.durationMs * 100) / 100;
  const requestBody = {
    route: payload.route,
    metric,
    duration_ms: normalizedDuration,
    status: payload.status ?? "ok",
    sample_size: normalizeSampleSize(payload.sampleSize),
    metadata: normalizeMetadata(payload.metadata),
  };

  void fetch("/api/telemetry/perf", {
    method: "POST",
    cache: "no-store",
    keepalive: true,
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(requestBody),
  }).catch(() => {
    // best effort telemetry
  });
}

function normalizeSampleSize(value: number | undefined): number {
  if (typeof value !== "number" || !isFinite(value)) {
    return 0;
  }
  if (value < 0) {
    return 0;
  }
  if (value > 100_000) {
    return 100_000;
  }
  return Math.round(value);
}

function normalizeMetadata(metadata: RoutePerfPayload["metadata"]): Record<string, string | number | boolean> {
  const out: Record<string, string | number | boolean> = {};
  if (!metadata) {
    return out;
  }

  const entries = Object.entries(metadata).slice(0, 16);
  for (const [key, rawValue] of entries) {
    const normalizedKey = normalizeMetadataKey(key);
    if (!normalizedKey) continue;
    if (rawValue == null) continue;
    if (typeof rawValue === "string") {
      const trimmed = rawValue.trim();
      if (!trimmed) continue;
      out[normalizedKey] = trimmed.slice(0, 180);
      continue;
    }
    if (typeof rawValue === "number") {
      if (!isFinite(rawValue)) continue;
      out[normalizedKey] = Math.round(rawValue * 100) / 100;
      continue;
    }
    out[normalizedKey] = rawValue;
  }
  return out;
}

function normalizeThrottleWindow(value: number | undefined): number {
  if (typeof value !== "number" || !isFinite(value)) {
    return sendThrottleMs;
  }
  if (value <= 0) {
    return 0;
  }
  return Math.min(Math.round(value), 120_000);
}

function normalizeMetadataKey(key: string): string {
  const trimmed = key.trim().toLowerCase();
  if (!trimmed) return "";
  const normalized = trimmed.replace(/[^a-z0-9_]+/g, "_").replace(/^_+|_+$/g, "");
  return normalized.slice(0, 48);
}

function isAutomatedBrowser(): boolean {
  if (typeof navigator === "undefined") return false;
  const userAgent = navigator.userAgent || "";
  return /playwright|headless/i.test(userAgent);
}
