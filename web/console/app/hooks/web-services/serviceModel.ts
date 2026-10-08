import type { WebService, WebServiceAltURL, WebServiceHealthPoint, WebServiceHealthSummary } from "./types";
import { asObject, asString, asFiniteNumber } from "./payloadValues";

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

function webServiceIdentityKey(hostAssetID: string, serviceID: string): string {
  return `${hostAssetID}::${serviceID}`;
}

export function reconcileWebServiceList(
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
