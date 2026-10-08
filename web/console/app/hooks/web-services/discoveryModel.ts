import type { URLGroupingSuggestion, WebServiceDiscoveryHostStat, WebServiceDiscoverySourceStat, WebServiceDiscoveryStats } from "./types";
import { asObject, asString, asFiniteNumber, asBoolean, asRecordNumber } from "./payloadValues";

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

export function normalizeDiscoveryHostStatList(value: unknown): WebServiceDiscoveryHostStat[] {
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

export function normalizeSuggestionList(value: unknown): URLGroupingSuggestion[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map(normalizeSuggestion)
    .filter((entry): entry is URLGroupingSuggestion => entry !== null);
}

export function areDiscoveryStatsEqual(
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

export function areSuggestionListsEqual(
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
