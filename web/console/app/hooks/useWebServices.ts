"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { reportRoutePerfMetric } from "./useRoutePerfTelemetry";

export interface WebServiceAltURL {
  id: string;
  web_service_id: string;
  url: string;
  source: string; // "auto" | "manual" | "suggestion_accepted"
  created_at: string;
}

export interface WebService {
  id: string;
  service_key: string;
  name: string;
  category: string;
  url: string;
  source: string;
  status: string;
  response_ms: number;
  container_id?: string;
  service_unit?: string;
  host_asset_id: string;
  icon_key: string;
  metadata?: Record<string, string>;
  health?: WebServiceHealthSummary;
  alt_urls?: WebServiceAltURL[];
}

export interface WebServiceHealthPoint {
  at: string;
  status: string;
  response_ms?: number;
}

export interface WebServiceHealthSummary {
  window: string;
  checks: number;
  up_checks: number;
  uptime_percent: number;
  last_checked_at?: string;
  last_change_at?: string;
  recent?: WebServiceHealthPoint[];
}

export interface WebServiceDiscoverySourceStat {
  enabled: boolean;
  duration_ms: number;
  services_found: number;
}

export interface WebServiceDiscoveryStats {
  collected_at: string;
  cycle_duration_ms: number;
  total_services: number;
  sources?: Record<string, WebServiceDiscoverySourceStat>;
  final_source_count?: Record<string, number>;
}

export interface WebServiceDiscoveryHostStat {
  host_asset_id: string;
  last_seen: string;
  discovery: WebServiceDiscoveryStats;
}

export interface URLGroupingSuggestion {
  id: string;
  base_service_url: string;
  base_service_name: string;
  base_icon_key: string;
  suggested_url: string;
  confidence: number;
}

interface UseWebServicesOptions {
  host?: string;
  includeHidden?: boolean;
  pollInterval?: number;
  detailLevel?: "compact" | "full";
}

function asObject(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function asString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function asFiniteNumber(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function asBoolean(value: unknown): boolean {
  return value === true;
}

function asRecordString(value: unknown): Record<string, string> | undefined {
  const raw = asObject(value);
  if (!raw) {
    return undefined;
  }
  const normalized: Record<string, string> = {};
  for (const [key, entry] of Object.entries(raw)) {
    if (typeof entry === "string") {
      normalized[key] = entry;
    }
  }
  return Object.keys(normalized).length > 0 ? normalized : undefined;
}

function asRecordNumber(value: unknown): Record<string, number> | undefined {
  const raw = asObject(value);
  if (!raw) {
    return undefined;
  }
  const normalized: Record<string, number> = {};
  for (const [key, entry] of Object.entries(raw)) {
    if (typeof entry === "number" && Number.isFinite(entry)) {
      normalized[key] = entry;
    }
  }
  return Object.keys(normalized).length > 0 ? normalized : undefined;
}

function normalizeHealthPoint(value: unknown): WebServiceHealthPoint | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  const at = asString(raw.at);
  const status = asString(raw.status);
  if (!at && !status) {
    return null;
  }
  return {
    at,
    status,
    response_ms: typeof raw.response_ms === "number" && Number.isFinite(raw.response_ms)
      ? raw.response_ms
      : undefined,
  };
}

function normalizeHealthSummary(value: unknown): WebServiceHealthSummary | undefined {
  const raw = asObject(value);
  if (!raw) {
    return undefined;
  }
  return {
    window: asString(raw.window),
    checks: asFiniteNumber(raw.checks),
    up_checks: asFiniteNumber(raw.up_checks),
    uptime_percent: asFiniteNumber(raw.uptime_percent),
    last_checked_at: asString(raw.last_checked_at) || undefined,
    last_change_at: asString(raw.last_change_at) || undefined,
    recent: Array.isArray(raw.recent)
      ? raw.recent.map(normalizeHealthPoint).filter((entry): entry is WebServiceHealthPoint => entry !== null)
      : undefined,
  };
}

function normalizeAltURL(value: unknown): WebServiceAltURL | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  const id = asString(raw.id);
  const url = asString(raw.url);
  if (!id && !url) {
    return null;
  }
  return {
    id,
    web_service_id: asString(raw.web_service_id),
    url,
    source: asString(raw.source),
    created_at: asString(raw.created_at),
  };
}

function fallbackServiceKey(name: string, id: string): string {
  const seed = name || id;
  if (!seed) {
    return "";
  }
  return seed.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

function normalizeWebService(value: unknown): WebService | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  const id = asString(raw.id);
  const name = asString(raw.name) || id || "Service";
  return {
    id,
    service_key: asString(raw.service_key) || fallbackServiceKey(name, id),
    name,
    category: asString(raw.category) || "Other",
    url: asString(raw.url),
    source: asString(raw.source),
    status: asString(raw.status) || "unknown",
    response_ms: asFiniteNumber(raw.response_ms),
    container_id: asString(raw.container_id) || undefined,
    service_unit: asString(raw.service_unit) || undefined,
    host_asset_id: asString(raw.host_asset_id),
    icon_key: asString(raw.icon_key),
    metadata: asRecordString(raw.metadata),
    health: normalizeHealthSummary(raw.health),
    alt_urls: Array.isArray(raw.alt_urls)
      ? raw.alt_urls.map(normalizeAltURL).filter((entry): entry is WebServiceAltURL => entry !== null)
      : undefined,
  };
}

function normalizeWebServiceList(value: unknown): WebService[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.map(normalizeWebService).filter((service): service is WebService => service !== null);
}

function normalizeDiscoverySourceStat(value: unknown): WebServiceDiscoverySourceStat | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  return {
    enabled: asBoolean(raw.enabled),
    duration_ms: asFiniteNumber(raw.duration_ms),
    services_found: asFiniteNumber(raw.services_found),
  };
}

function normalizeDiscoverySourceStatMap(
  value: unknown
): Record<string, WebServiceDiscoverySourceStat> | undefined {
  const raw = asObject(value);
  if (!raw) {
    return undefined;
  }
  const normalized: Record<string, WebServiceDiscoverySourceStat> = {};
  for (const [key, entry] of Object.entries(raw)) {
    const stat = normalizeDiscoverySourceStat(entry);
    if (stat) {
      normalized[key] = stat;
    }
  }
  return Object.keys(normalized).length > 0 ? normalized : undefined;
}

function normalizeDiscoveryStats(value: unknown): WebServiceDiscoveryStats | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  return {
    collected_at: asString(raw.collected_at),
    cycle_duration_ms: asFiniteNumber(raw.cycle_duration_ms),
    total_services: asFiniteNumber(raw.total_services),
    sources: normalizeDiscoverySourceStatMap(raw.sources),
    final_source_count: asRecordNumber(raw.final_source_count),
  };
}

function normalizeDiscoveryHostStat(value: unknown): WebServiceDiscoveryHostStat | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  const discovery = normalizeDiscoveryStats(raw.discovery);
  if (!discovery) {
    return null;
  }
  return {
    host_asset_id: asString(raw.host_asset_id),
    last_seen: asString(raw.last_seen),
    discovery,
  };
}

function normalizeDiscoveryHostStatList(value: unknown): WebServiceDiscoveryHostStat[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map(normalizeDiscoveryHostStat)
    .filter((entry): entry is WebServiceDiscoveryHostStat => entry !== null);
}

function normalizeSuggestion(value: unknown): URLGroupingSuggestion | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  const id = asString(raw.id);
  const suggestedURL = asString(raw.suggested_url);
  if (!id && !suggestedURL) {
    return null;
  }
  return {
    id,
    base_service_url: asString(raw.base_service_url),
    base_service_name: asString(raw.base_service_name),
    base_icon_key: asString(raw.base_icon_key),
    suggested_url: suggestedURL,
    confidence: asFiniteNumber(raw.confidence),
  };
}

function normalizeSuggestionList(value: unknown): URLGroupingSuggestion[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map(normalizeSuggestion)
    .filter((entry): entry is URLGroupingSuggestion => entry !== null);
}

function normalizeWebServiceOverride(value: unknown): WebServiceOverride | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  const hostAssetID = asString(raw.host_asset_id);
  const serviceID = asString(raw.service_id);
  if (!hostAssetID && !serviceID) {
    return null;
  }
  return {
    host_asset_id: hostAssetID,
    service_id: serviceID,
    name_override: asString(raw.name_override) || undefined,
    category_override: asString(raw.category_override) || undefined,
    url_override: asString(raw.url_override) || undefined,
    icon_key_override: asString(raw.icon_key_override) || undefined,
    tags_override: asString(raw.tags_override) || undefined,
    hidden: asBoolean(raw.hidden),
    updated_at: asString(raw.updated_at) || undefined,
  };
}

function normalizeWebServiceOverrideList(value: unknown): WebServiceOverride[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map(normalizeWebServiceOverride)
    .filter((entry): entry is WebServiceOverride => entry !== null);
}

function normalizeCustomServiceIcon(value: unknown): ServiceCustomIcon | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  const id = asString(raw.id);
  const name = asString(raw.name);
  if (!id && !name) {
    return null;
  }
  return {
    id,
    name,
    data_url: asString(raw.data_url),
    created_at: asString(raw.created_at) || undefined,
    updated_at: asString(raw.updated_at) || undefined,
  };
}

function normalizeCustomServiceIconList(value: unknown): ServiceCustomIcon[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map(normalizeCustomServiceIcon)
    .filter((entry): entry is ServiceCustomIcon => entry !== null);
}

const labtetherServiceKey = "labtether";
const bootstrapServiceKey = "bootstrap";
const labtetherAPIName = "labtether api";
const labtetherComponentMetadataKey = "labtether_component";
const labtetherAPIComponent = "api";
const labtetherConsoleComponent = "console";

function servicePortFromURL(raw: string): number {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return 0;
  }
  try {
    const parsed = new URL(trimmed);
    const port = parsed.port.trim();
    if (port === "") {
      return 0;
    }
    const value = Number.parseInt(port, 10);
    if (!Number.isFinite(value) || value <= 0 || value > 65535) {
      return 0;
    }
    return value;
  } catch {
    return 0;
  }
}

function isBootstrapService(service: WebService): boolean {
  const serviceKey = service.service_key.trim().toLowerCase();
  if (serviceKey === bootstrapServiceKey) {
    return true;
  }

  if (serviceKey === labtetherServiceKey) {
    const component = service.metadata?.[labtetherComponentMetadataKey]?.trim()?.toLowerCase() ?? "";
    if (component === labtetherAPIComponent) {
      return true;
    }
    if (component === labtetherConsoleComponent) {
      return false;
    }

    const port = servicePortFromURL(service.url);
    if (port === 8080 || port === 8443) {
      return true;
    }
    const healthPath = service.metadata?.health_path?.trim()?.toLowerCase() ?? "";
    if (healthPath === "/healthz" || healthPath === "/version") {
      return true;
    }
  }

  return service.name.trim().toLowerCase() === labtetherAPIName;
}

export interface ManualWebServiceInput {
  host_asset_id: string;
  name: string;
  category: string;
  url: string;
  icon_key?: string;
  metadata?: Record<string, string>;
}

export interface WebServiceOverrideInput {
  host_asset_id: string;
  service_id: string;
  name_override?: string;
  category_override?: string;
  url_override?: string;
  icon_key_override?: string;
  tags_override?: string;
  hidden: boolean;
}

export interface WebServiceOverride {
  host_asset_id: string;
  service_id: string;
  name_override?: string;
  category_override?: string;
  url_override?: string;
  icon_key_override?: string;
  tags_override?: string;
  hidden: boolean;
  updated_at?: string;
}

export interface ServiceCustomIconInput {
  name: string;
  data_url: string;
}

export interface ServiceCustomIconRenameInput {
  id: string;
  name: string;
}

export interface ServiceCustomIcon {
  id: string;
  name: string;
  data_url: string;
  created_at?: string;
  updated_at?: string;
}

export function useWebServices(options: UseWebServicesOptions = {}) {
  const { host, includeHidden = false, pollInterval = 30000, detailLevel = "full" } = options;
  const [services, setServices] = useState<WebService[]>([]);
  const [discoveryStats, setDiscoveryStats] = useState<WebServiceDiscoveryHostStat[]>([]);
  const [suggestions, setSuggestions] = useState<URLGroupingSuggestion[]>([]);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const servicesRef = useRef<WebService[]>([]);
  const discoveryStatsRef = useRef<WebServiceDiscoveryHostStat[]>([]);
  const suggestionsRef = useRef<URLGroupingSuggestion[]>([]);

  const fetchServicePayload = useCallback(async ({
    detail,
    hostAssetID,
    includeHiddenServices,
    serviceID,
    signal,
  }: {
    detail: "compact" | "full";
    hostAssetID?: string;
    includeHiddenServices: boolean;
    serviceID?: string;
    signal?: AbortSignal;
  }) => {
    const params = new URLSearchParams();
    if (hostAssetID) params.set("host", hostAssetID);
    if (includeHiddenServices) params.set("include_hidden", "true");
    if (detail !== "full") params.set("detail", detail);
    if (serviceID) params.set("service_id", serviceID);
    const url = `/api/services/web${params.toString() ? "?" + params.toString() : ""}`;

    const response = await fetch(url, {
      cache: "no-store",
      signal,
    });
    const data = (await safeJSON(response)) as {
      services?: unknown;
      discovery_stats?: unknown;
      suggestions?: unknown;
      error?: string;
    } | null;
    const reconciledServices = reconcileWebServiceList(
      data?.services,
      servicesRef.current,
      includeHiddenServices
    );
    const normalizedDiscoveryStats = normalizeDiscoveryHostStatList(data?.discovery_stats);
    const nextDiscoveryStats = areDiscoveryStatsEqual(discoveryStatsRef.current, normalizedDiscoveryStats)
      ? discoveryStatsRef.current
      : normalizedDiscoveryStats;
    const normalizedSuggestions = normalizeSuggestionList(data?.suggestions);
    const nextSuggestions = areSuggestionListsEqual(suggestionsRef.current, normalizedSuggestions)
      ? suggestionsRef.current
      : normalizedSuggestions;

    return {
      response,
      nextServices: reconciledServices.next,
      reusedServices: reconciledServices.reused,
      changedServices: reconciledServices.changed,
      nextDiscoveryStats,
      nextSuggestions,
      errorMessage: data?.error,
    };
  }, []);

  const fetchServices = useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    const startedAt = typeof performance !== "undefined" ? performance.now() : Date.now();

    try {
      const {
        response: res,
        nextServices,
        reusedServices,
        changedServices,
        nextDiscoveryStats,
        nextSuggestions,
      } = await fetchServicePayload({
        detail: detailLevel,
        hostAssetID: host,
        includeHiddenServices: includeHidden,
        signal: controller.signal,
      });

      if (res.ok) {
        if (servicesRef.current !== nextServices) {
          servicesRef.current = nextServices;
          setServices(nextServices);
        }
        if (discoveryStatsRef.current !== nextDiscoveryStats) {
          discoveryStatsRef.current = nextDiscoveryStats;
          setDiscoveryStats(nextDiscoveryStats);
        }
        if (suggestionsRef.current !== nextSuggestions) {
          suggestionsRef.current = nextSuggestions;
          setSuggestions(nextSuggestions);
        }
        reportRoutePerfMetric({
          route: "services",
          metric: "request.services_fetch",
          durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
          sampleSize: nextServices.length,
          metadata: {
            host_filtered: Boolean(host),
            include_hidden: includeHidden,
            detail_level: detailLevel,
            discovery_hosts: nextDiscoveryStats.length,
            suggestions: nextSuggestions.length,
            reused_services: reusedServices,
            changed_services: changedServices,
          },
        });
        const scheduleAfterPaint = typeof window.requestAnimationFrame === "function"
          ? window.requestAnimationFrame.bind(window)
          : (callback: FrameRequestCallback) => window.setTimeout(() => callback(performance.now()), 0);
        scheduleAfterPaint(() => {
          reportRoutePerfMetric({
            route: "services",
            metric: "render.services_results",
            durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
            sampleSize: nextServices.length,
            metadata: {
              host_filtered: Boolean(host),
              include_hidden: includeHidden,
              detail_level: detailLevel,
              discovery_hosts: nextDiscoveryStats.length,
              suggestions: nextSuggestions.length,
              reused_services: reusedServices,
              changed_services: changedServices,
            },
          });
        });
        setError(null);
      } else {
        reportRoutePerfMetric({
          route: "services",
          metric: "request.services_fetch",
          durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
          status: "error",
          metadata: {
            http_status: res.status,
            host_filtered: Boolean(host),
            include_hidden: includeHidden,
            detail_level: detailLevel,
          },
        });
        setError(`HTTP ${res.status}`);
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      reportRoutePerfMetric({
        route: "services",
        metric: "request.services_fetch",
        durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
        status: "error",
        metadata: {
          host_filtered: Boolean(host),
          include_hidden: includeHidden,
          detail_level: detailLevel,
        },
      });
      setError(err instanceof Error ? err.message : "Failed to fetch services");
    } finally {
      setLoading(false);
    }
  }, [detailLevel, fetchServicePayload, host, includeHidden]);

  useEffect(() => {
    let interval: ReturnType<typeof setInterval> | null = null;

    const start = () => {
      void fetchServices();
      if (interval === null) {
        interval = setInterval(fetchServices, pollInterval);
      }
    };

    const stop = () => {
      if (interval !== null) {
        clearInterval(interval);
        interval = null;
      }
      abortRef.current?.abort();
    };

    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        start();
        return;
      }
      stop();
    };

    if (document.visibilityState === "visible") {
      start();
    }

    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      stop();
    };
  }, [fetchServices, pollInterval]);

  const sync = useCallback(
    async (targetHost?: string) => {
      setSyncing(true);
      try {
        const params = new URLSearchParams();
        const hostValue = targetHost ?? host;
        if (hostValue) params.set("host", hostValue);
        const url = `/api/services/web/sync${params.toString() ? "?" + params.toString() : ""}`;

        const res = await fetch(url, {
          method: "POST",
          cache: "no-store",
        });
        if (!res.ok) {
          const payload = (await safeJSON(res)) as { error?: string } | null;
          throw new Error(payload?.error ?? `HTTP ${res.status}`);
        }
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to trigger service sync");
        throw err;
      } finally {
        setSyncing(false);
      }
    },
    [host]
  );

  const createManualService = useCallback(async (input: ManualWebServiceInput) => {
    const res = await fetch("/api/services/web/manual", {
      method: "POST",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(input),
    });
    if (!res.ok) {
      const payload = (await safeJSON(res)) as { error?: string } | null;
      throw new Error(payload?.error ?? `HTTP ${res.status}`);
    }
  }, []);

  const updateManualService = useCallback(async (id: string, patch: Partial<ManualWebServiceInput>) => {
    const res = await fetch(`/api/services/web/manual/${encodeURIComponent(id)}`, {
      method: "PATCH",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(patch),
    });
    if (!res.ok) {
      const payload = (await safeJSON(res)) as { error?: string } | null;
      throw new Error(payload?.error ?? `HTTP ${res.status}`);
    }
  }, []);

  const deleteManualService = useCallback(async (id: string) => {
    const res = await fetch(`/api/services/web/manual/${encodeURIComponent(id)}`, {
      method: "DELETE",
      cache: "no-store",
    });
    if (!res.ok && res.status !== 204) {
      const payload = (await safeJSON(res)) as { error?: string } | null;
      throw new Error(payload?.error ?? `HTTP ${res.status}`);
    }
  }, []);

  const saveServiceOverride = useCallback(async (input: WebServiceOverrideInput) => {
    const res = await fetch("/api/services/web/overrides", {
      method: "POST",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(input),
    });
    if (!res.ok) {
      const payload = (await safeJSON(res)) as { error?: string } | null;
      throw new Error(payload?.error ?? `HTTP ${res.status}`);
    }
  }, []);

  const listServiceOverrides = useCallback(async (hostAssetID?: string) => {
    const params = new URLSearchParams();
    if (hostAssetID) {
      params.set("host", hostAssetID);
    }
    const url = `/api/services/web/overrides${params.toString() ? "?" + params.toString() : ""}`;
    const res = await fetch(url, {
      cache: "no-store",
    });
    if (!res.ok) {
      const payload = (await safeJSON(res)) as { error?: string } | null;
      throw new Error(payload?.error ?? `HTTP ${res.status}`);
    }
    const data = (await safeJSON(res)) as { overrides?: WebServiceOverride[] } | null;
    return normalizeWebServiceOverrideList(data?.overrides);
  }, []);

  const deleteServiceOverride = useCallback(async (hostAssetID: string, serviceID: string) => {
    const params = new URLSearchParams();
    if (hostAssetID) {
      params.set("host", hostAssetID);
    }
    if (serviceID) {
      params.set("service_id", serviceID);
    }
    const url = `/api/services/web/overrides${params.toString() ? "?" + params.toString() : ""}`;
    const res = await fetch(url, {
      method: "DELETE",
      cache: "no-store",
    });
    if (!res.ok && res.status !== 204) {
      const payload = (await safeJSON(res)) as { error?: string } | null;
      throw new Error(payload?.error ?? `HTTP ${res.status}`);
    }
  }, []);

  const listCustomServiceIcons = useCallback(async () => {
    const res = await fetch("/api/services/web/icon-library", {
      cache: "no-store",
    });
    if (!res.ok) {
      const payload = (await safeJSON(res)) as { error?: string } | null;
      throw new Error(payload?.error ?? `HTTP ${res.status}`);
    }
    const data = (await safeJSON(res)) as { icons?: unknown } | null;
    return normalizeCustomServiceIconList(data?.icons);
  }, []);

  const createCustomServiceIcon = useCallback(async (input: ServiceCustomIconInput) => {
    const res = await fetch("/api/services/web/icon-library", {
      method: "POST",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(input),
    });
    const payload = (await safeJSON(res)) as { icon?: unknown; error?: string } | null;
    if (!res.ok) {
      throw new Error(payload?.error ?? `HTTP ${res.status}`);
    }
    const icon = normalizeCustomServiceIcon(payload?.icon);
    if (!icon) {
      throw new Error("service icon create response missing icon");
    }
    return icon;
  }, []);

  const deleteCustomServiceIcon = useCallback(async (id: string) => {
    const params = new URLSearchParams();
    params.set("id", id);
    const res = await fetch(`/api/services/web/icon-library?${params.toString()}`, {
      method: "DELETE",
      cache: "no-store",
    });
    if (!res.ok && res.status !== 204) {
      const payload = (await safeJSON(res)) as { error?: string } | null;
      throw new Error(payload?.error ?? `HTTP ${res.status}`);
    }
  }, []);

  const renameCustomServiceIcon = useCallback(async (input: ServiceCustomIconRenameInput) => {
    const params = new URLSearchParams();
    params.set("id", input.id);
    const res = await fetch(`/api/services/web/icon-library?${params.toString()}`, {
      method: "PATCH",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ name: input.name }),
    });
    const payload = (await safeJSON(res)) as { icon?: unknown; error?: string } | null;
    if (!res.ok) {
      throw new Error(payload?.error ?? `HTTP ${res.status}`);
    }
    const icon = normalizeCustomServiceIcon(payload?.icon);
    if (!icon) {
      throw new Error("service icon rename response missing icon");
    }
    return icon;
  }, []);

  const loadServiceDetails = useCallback(async (serviceID: string, hostAssetID?: string) => {
    const startedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
    const {
      response,
      nextServices,
      errorMessage,
    } = await fetchServicePayload({
      detail: "full",
      hostAssetID: hostAssetID ?? host,
      includeHiddenServices: includeHidden,
      serviceID,
    });
    if (!response.ok) {
      throw new Error(errorMessage ?? `HTTP ${response.status}`);
    }
    reportRoutePerfMetric({
      route: "services",
      metric: "request.service_detail_fetch",
      durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
      sampleSize: nextServices.length,
      metadata: {
        host_filtered: Boolean(hostAssetID ?? host),
        include_hidden: includeHidden,
      },
    });
    return nextServices[0] ?? null;
  }, [fetchServicePayload, host, includeHidden]);

  return {
    services,
    discoveryStats,
    suggestions,
    loading,
    syncing,
    error,
    refresh: fetchServices,
    sync,
    createManualService,
    updateManualService,
    deleteManualService,
    saveServiceOverride,
    listServiceOverrides,
    deleteServiceOverride,
    listCustomServiceIcons,
    createCustomServiceIcon,
    deleteCustomServiceIcon,
    renameCustomServiceIcon,
    loadServiceDetails,
  };
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

function webServiceIdentityKey(hostAssetID: string, serviceID: string): string {
  return `${hostAssetID}::${serviceID}`;
}

function reconcileWebServiceList(
  value: unknown,
  existing: WebService[],
  includeHiddenServices: boolean
): { next: WebService[]; reused: number; changed: number } {
  if (!Array.isArray(value)) {
    return existing.length === 0
      ? { next: existing, reused: 0, changed: 0 }
      : { next: [], reused: 0, changed: existing.length };
  }

  const existingByKey = new Map<string, WebService>();
  for (const service of existing) {
    existingByKey.set(webServiceIdentityKey(service.host_asset_id, service.id), service);
  }

  const next: WebService[] = [];
  let reused = 0;
  let changed = 0;
  for (const entry of value) {
    const raw = asObject(entry);
    if (!raw) {
      continue;
    }
    const existingService = existingByKey.get(
      webServiceIdentityKey(asString(raw.host_asset_id), asString(raw.id))
    );
    const service = reconcileWebService(raw, existingService);
    if (!service) {
      continue;
    }
    if (!includeHiddenServices && isBootstrapService(service)) {
      continue;
    }

    const nextIndex = next.length;
    next.push(service);
    if (existing[nextIndex] === service) {
      reused += 1;
      continue;
    }
    changed += 1;
  }

  if (next.length !== existing.length) {
    changed += Math.abs(existing.length - next.length);
  }
  if (changed === 0 && next.length === existing.length) {
    return { next: existing, reused, changed: 0 };
  }
  return { next, reused, changed };
}

function reconcileWebService(
  raw: Record<string, unknown>,
  existing?: WebService
): WebService | null {
  const id = asString(raw.id);
  const name = asString(raw.name) || id || "Service";
  const service = {
    id,
    service_key: asString(raw.service_key) || fallbackServiceKey(name, id),
    name,
    category: asString(raw.category) || "Other",
    url: asString(raw.url),
    source: asString(raw.source),
    status: asString(raw.status) || "unknown",
    response_ms: asFiniteNumber(raw.response_ms),
    container_id: asString(raw.container_id) || undefined,
    service_unit: asString(raw.service_unit) || undefined,
    host_asset_id: asString(raw.host_asset_id),
    icon_key: asString(raw.icon_key),
    metadata: reconcileStringMap(raw.metadata, existing?.metadata),
    health: reconcileHealthSummary(raw.health, existing?.health),
    alt_urls: reconcileAltURLList(raw.alt_urls, existing?.alt_urls),
  } satisfies WebService;

  if (
    existing
    && existing.id === service.id
    && existing.service_key === service.service_key
    && existing.name === service.name
    && existing.category === service.category
    && existing.url === service.url
    && existing.source === service.source
    && existing.status === service.status
    && existing.response_ms === service.response_ms
    && existing.container_id === service.container_id
    && existing.service_unit === service.service_unit
    && existing.host_asset_id === service.host_asset_id
    && existing.icon_key === service.icon_key
    && existing.metadata === service.metadata
    && existing.health === service.health
    && existing.alt_urls === service.alt_urls
  ) {
    return existing;
  }

  return service;
}

function reconcileStringMap(
  value: unknown,
  existing?: Record<string, string>
): Record<string, string> | undefined {
  const raw = asObject(value);
  if (!raw) {
    return undefined;
  }

  let count = 0;
  let unchanged = existing !== undefined;
  for (const [key, entry] of Object.entries(raw)) {
    if (typeof entry !== "string") {
      continue;
    }
    count += 1;
    if (!unchanged) {
      continue;
    }
    if (!existing || existing[key] !== entry) {
      unchanged = false;
    }
  }
  if (count === 0) {
    return undefined;
  }
  if (unchanged && existing && Object.keys(existing).length === count) {
    return existing;
  }

  const normalized: Record<string, string> = {};
  for (const [key, entry] of Object.entries(raw)) {
    if (typeof entry === "string") {
      normalized[key] = entry;
    }
  }
  return normalized;
}

function reconcileHealthPointList(
  value: unknown,
  existing?: WebServiceHealthPoint[]
): WebServiceHealthPoint[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }

  const next: WebServiceHealthPoint[] = [];
  let changed = existing === undefined || existing.length !== value.length;
  for (let index = 0; index < value.length; index += 1) {
    const point = normalizeHealthPoint(value[index]);
    if (!point) {
      changed = true;
      continue;
    }
    next.push(point);
    const existingPoint = existing?.[next.length - 1];
    if (
      existingPoint
      && existingPoint.at === point.at
      && existingPoint.status === point.status
      && existingPoint.response_ms === point.response_ms
    ) {
      next[next.length - 1] = existingPoint;
      continue;
    }
    changed = true;
  }

  if (!changed && existing && next.length === existing.length) {
    return existing;
  }
  return next.length > 0 ? next : undefined;
}

function reconcileHealthSummary(
  value: unknown,
  existing?: WebServiceHealthSummary
): WebServiceHealthSummary | undefined {
  const raw = asObject(value);
  if (!raw) {
    return undefined;
  }

  const health = {
    window: asString(raw.window),
    checks: asFiniteNumber(raw.checks),
    up_checks: asFiniteNumber(raw.up_checks),
    uptime_percent: asFiniteNumber(raw.uptime_percent),
    last_checked_at: asString(raw.last_checked_at) || undefined,
    last_change_at: asString(raw.last_change_at) || undefined,
    recent: reconcileHealthPointList(raw.recent, existing?.recent),
  } satisfies WebServiceHealthSummary;

  if (
    existing
    && existing.window === health.window
    && existing.checks === health.checks
    && existing.up_checks === health.up_checks
    && existing.uptime_percent === health.uptime_percent
    && existing.last_checked_at === health.last_checked_at
    && existing.last_change_at === health.last_change_at
    && existing.recent === health.recent
  ) {
    return existing;
  }

  return health;
}

function reconcileAltURLList(
  value: unknown,
  existing?: WebServiceAltURL[]
): WebServiceAltURL[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }

  const next: WebServiceAltURL[] = [];
  let changed = existing === undefined || existing.length !== value.length;
  for (let index = 0; index < value.length; index += 1) {
    const altURL = normalizeAltURL(value[index]);
    if (!altURL) {
      changed = true;
      continue;
    }
    next.push(altURL);
    const existingAltURL = existing?.[next.length - 1];
    if (
      existingAltURL
      && existingAltURL.id === altURL.id
      && existingAltURL.web_service_id === altURL.web_service_id
      && existingAltURL.url === altURL.url
      && existingAltURL.source === altURL.source
      && existingAltURL.created_at === altURL.created_at
    ) {
      next[next.length - 1] = existingAltURL;
      continue;
    }
    changed = true;
  }

  if (!changed && existing && next.length === existing.length) {
    return existing;
  }
  return next.length > 0 ? next : undefined;
}

function areDiscoveryStatsEqual(
  left: WebServiceDiscoveryHostStat[],
  right: WebServiceDiscoveryHostStat[]
): boolean {
  if (left === right) {
    return true;
  }
  if (left.length !== right.length) {
    return false;
  }
  for (let index = 0; index < left.length; index += 1) {
    if (!areDiscoveryHostStatsEqual(left[index], right[index])) {
      return false;
    }
  }
  return true;
}

function areDiscoveryHostStatsEqual(
  left: WebServiceDiscoveryHostStat,
  right: WebServiceDiscoveryHostStat
): boolean {
  return (
    left.host_asset_id === right.host_asset_id
    && left.last_seen === right.last_seen
    && areDiscoveryCycleStatsEqual(left.discovery, right.discovery)
  );
}

function areDiscoveryCycleStatsEqual(
  left: WebServiceDiscoveryStats,
  right: WebServiceDiscoveryStats
): boolean {
  return (
    left.collected_at === right.collected_at
    && left.cycle_duration_ms === right.cycle_duration_ms
    && left.total_services === right.total_services
    && areRecordOfNumbersEqual(left.final_source_count, right.final_source_count)
    && areSourceStatsMapEqual(left.sources, right.sources)
  );
}

function areRecordOfNumbersEqual(
  left?: Record<string, number>,
  right?: Record<string, number>
): boolean {
  const leftEntries = Object.entries(left ?? {}).sort(([a], [b]) => a.localeCompare(b));
  const rightEntries = Object.entries(right ?? {}).sort(([a], [b]) => a.localeCompare(b));
  if (leftEntries.length !== rightEntries.length) {
    return false;
  }
  for (let index = 0; index < leftEntries.length; index += 1) {
    if (leftEntries[index][0] !== rightEntries[index][0]) {
      return false;
    }
    if (leftEntries[index][1] !== rightEntries[index][1]) {
      return false;
    }
  }
  return true;
}

function areSourceStatsMapEqual(
  left?: Record<string, WebServiceDiscoverySourceStat>,
  right?: Record<string, WebServiceDiscoverySourceStat>
): boolean {
  const leftEntries = Object.entries(left ?? {}).sort(([a], [b]) => a.localeCompare(b));
  const rightEntries = Object.entries(right ?? {}).sort(([a], [b]) => a.localeCompare(b));
  if (leftEntries.length !== rightEntries.length) {
    return false;
  }
  for (let index = 0; index < leftEntries.length; index += 1) {
    if (leftEntries[index][0] !== rightEntries[index][0]) {
      return false;
    }
    const leftValue = leftEntries[index][1];
    const rightValue = rightEntries[index][1];
    if (
      leftValue.enabled !== rightValue.enabled
      || leftValue.duration_ms !== rightValue.duration_ms
      || leftValue.services_found !== rightValue.services_found
    ) {
      return false;
    }
  }
  return true;
}

function areSuggestionListsEqual(
  left: URLGroupingSuggestion[],
  right: URLGroupingSuggestion[]
): boolean {
  if (left === right) {
    return true;
  }
  if (left.length !== right.length) {
    return false;
  }
  for (let index = 0; index < left.length; index += 1) {
    const l = left[index];
    const r = right[index];
    if (
      l.id !== r.id
      || l.base_service_url !== r.base_service_url
      || l.base_service_name !== r.base_service_name
      || l.base_icon_key !== r.base_icon_key
      || l.suggested_url !== r.suggested_url
      || l.confidence !== r.confidence
    ) {
      return false;
    }
  }
  return true;
}
