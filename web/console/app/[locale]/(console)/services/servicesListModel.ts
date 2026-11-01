import type { WebService, WebServiceHealthPoint } from "../../../hooks/useWebServices";
import {
  CATEGORY_ORDER,
  matchesServiceHealthFilter,
  serviceLayoutKey,
  serviceSortCompare,
  type ServiceHealthFilter,
  type ServiceLayoutState,
} from "./servicesLayoutHelpers";

// The 2.0 Services list: which services show (text, chips and the Filters menu), their order and their groups.

export type ServiceShow = "all" | "down" | "hidden";
export type ServiceGrouping = "none" | "category" | "host";
export const SERVICE_GROUPINGS: ServiceGrouping[] = ["none", "category", "host"];
export type ServiceSortKey = "name" | "latency" | "most_unstable" | "uptime_high" | "recently_changed";
export const SERVICE_SORT_KEYS: ServiceSortKey[] = ["name", "latency", "most_unstable", "uptime_high", "recently_changed"];
export type ServiceSort = { key: ServiceSortKey; dir: "asc" | "desc" };
export const DEFAULT_SORT: ServiceSort = { key: "name", dir: "asc" };
/** Answers slower than this read amber in the Latency column. */
export const SLOW_MS = 100;

export type ServiceFilters = {
  query: string;
  show: ServiceShow;
  status: "all" | "up" | "down" | "unknown";
  category: string;
  host: string;
  source: string;
  health: ServiceHealthFilter;
};

export const NO_FILTERS: ServiceFilters = {
  query: "",
  show: "all",
  status: "all",
  category: "all",
  host: "all",
  source: "all",
  health: "all",
};

export const isHidden = (service: WebService) => service.metadata?.hidden === "true";
export const isDown = (service: WebService) => service.status === "down";
export const isManual = (service: WebService) => service.source === "manual" || service.metadata?.manual === "true";

/** "https://grafana.lan:3000/x" → "grafana.lan:3000". Anything that is not a URL stays as it is. */
export function serviceAddress(url: string): string {
  try {
    return new URL(url).host || url;
  } catch {
    return url;
  }
}

/**
 * The grey address after a name, host and port as the mockup shows them ("docker-01:3000", "nuc-01 · 10.20.0.1:443"):
 * the device name alone when the URL's host is that device, else the device and the URL's host. Ports are always
 * shown (443 and 80 included); anything that is not a URL stays as it is.
 */
export function serviceLine(url: string, hostName: string): string {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return [hostName, url].filter(Boolean).join(" · ");
  }
  const port = parsed.port || (parsed.protocol === "https:" ? "443" : parsed.protocol === "http:" ? "80" : "");
  const host = parsed.hostname;
  const sameHost = hostName !== "" && host.split(".")[0].toLowerCase() === hostName.toLowerCase();
  const address = `${sameHost ? hostName : host}${port ? `:${port}` : ""}`;
  return sameHost || !hostName ? address : `${hostName} · ${address}`;
}

/** Filters in the Filters menu that are not at their default (the chips and the text field are not counted). */
export function countMenuFilters(filters: ServiceFilters): number {
  let count = 0;
  if (filters.category !== "all") count += 1;
  if (filters.host !== "all") count += 1;
  if (filters.source !== "all") count += 1;
  if (filters.health !== "all") count += 1;
  if (filters.status !== "all" && filters.status !== "down") count += 1;
  return count;
}

/** True while anything narrows the list. */
export function isFiltering(filters: ServiceFilters): boolean {
  return filters.query.trim() !== "" || filters.show !== "all" || filters.status !== "all" || countMenuFilters(filters) > 0;
}

function matchesQuery(service: WebService, query: string, hostName: string): boolean {
  if (!query) return true;
  const haystack = [
    service.name,
    hostName,
    serviceAddress(service.url),
    service.url,
    service.category,
    service.metadata?.user_tags ?? "",
  ].join(" ").toLowerCase();
  return query.split(/\s+/).every((word) => haystack.includes(word));
}

/**
 * The services the list shows. Hidden services show only under the Hidden chip; the Down chip and the Status
 * filter share one value. hostNames maps a host asset id to its device name (for the text filter).
 */
export function filterServices(
  services: WebService[],
  filters: ServiceFilters,
  hostNames: Map<string, string>,
  nowMS = Date.now(),
): WebService[] {
  const query = filters.query.trim().toLowerCase();
  return services.filter((service) => {
    if (filters.show === "hidden" ? !isHidden(service) : isHidden(service)) return false;
    if (filters.show === "down" && !isDown(service)) return false;
    if (filters.status !== "all" && service.status !== filters.status) return false;
    if (filters.category !== "all" && service.category !== filters.category) return false;
    if (filters.host !== "all" && service.host_asset_id !== filters.host) return false;
    if (filters.source !== "all" && service.source !== filters.source) return false;
    if (!matchesServiceHealthFilter(service, filters.health, nowMS)) return false;
    return matchesQuery(service, query, hostNames.get(service.host_asset_id) ?? "");
  });
}

/** How many services each chip would show (Down and All leave hidden services out). */
export function chipCounts(services: WebService[]): Record<ServiceShow, number> {
  const counts: Record<ServiceShow, number> = { all: 0, down: 0, hidden: 0 };
  for (const service of services) {
    if (isHidden(service)) {
      counts.hidden += 1;
      continue;
    }
    counts.all += 1;
    if (isDown(service)) counts.down += 1;
  }
  return counts;
}

/** Down services sort last by latency; up services that report no time sort just before them. */
function latencyValue(service: WebService): number {
  if (isDown(service)) return Number.POSITIVE_INFINITY;
  return service.response_ms > 0 ? service.response_ms : Number.MAX_SAFE_INTEGER;
}

function byName(left: WebService, right: WebService): number {
  return left.name.localeCompare(right.name) || left.url.localeCompare(right.url);
}

/**
 * Sorts a copy. By name, down services lead (in either direction) so problems stay on top; the stability sorts
 * keep their own fixed direction.
 */
export function sortServices(services: WebService[], sort: ServiceSort): WebService[] {
  const sign = sort.dir === "desc" ? -1 : 1;
  const list = [...services];
  if (sort.key === "name") {
    return list.sort((a, b) => Number(isDown(b)) - Number(isDown(a)) || sign * byName(a, b));
  }
  if (sort.key === "latency") {
    return list.sort((a, b) => sign * (latencyValue(a) - latencyValue(b)) || byName(a, b));
  }
  const mode = sort.key;
  return list.sort((a, b) => serviceSortCompare(a, b, mode));
}

/** Puts services in the order someone arranged (by layout key); services not in that order keep theirs, after. */
export function orderByLayout(services: WebService[], order: string[]): WebService[] {
  if (order.length === 0) return services;
  const byKey = new Map(services.map((service) => [serviceLayoutKey(service), service]));
  const arranged: WebService[] = [];
  for (const key of order) {
    const service = byKey.get(key);
    if (!service) continue;
    arranged.push(service);
    byKey.delete(key);
  }
  return [...arranged, ...services.filter((service) => byKey.has(serviceLayoutKey(service)))];
}

export type ServiceGroup = { key: string; title: string; services: WebService[] };

function categoryRank(category: string): number {
  const index = CATEGORY_ORDER.indexOf(category);
  return index === -1 ? CATEGORY_ORDER.length : index;
}

/**
 * Cuts a sorted list into groups that keep its order. Categories follow the console's category order; hosts sort by
 * device name, and services without a device come last under an empty title. When arranged is given (grouped by
 * category, sorted by name), each category follows the order someone arranged.
 */
export function groupServices(
  services: WebService[],
  by: ServiceGrouping,
  hostNames: Map<string, string>,
  arranged?: ServiceLayoutState,
): ServiceGroup[] {
  if (by === "none") return services.length ? [{ key: "all", title: "", services }] : [];
  const groups = new Map<string, ServiceGroup>();
  for (const service of services) {
    const id = by === "category" ? service.category || "Other" : service.host_asset_id;
    const key = `${by}:${id}`;
    let group = groups.get(key);
    if (!group) {
      group = { key, title: by === "category" ? id : id ? (hostNames.get(id) ?? id) : "", services: [] };
      groups.set(key, group);
    }
    group.services.push(service);
  }
  const list = [...groups.values()];
  if (by === "category") {
    list.sort((a, b) => categoryRank(a.title) - categoryRank(b.title) || a.title.localeCompare(b.title));
    if (arranged) {
      for (const group of list) group.services = orderByLayout(group.services, arranged[group.title] ?? []);
    }
  } else {
    list.sort((a, b) => Number(a.title === "") - Number(b.title === "") || a.title.localeCompare(b.title));
  }
  return list;
}

export type Outage = { start: string; end?: string };

/**
 * Down periods in a service's recent checks, newest first. Each starts at the first down check and ends at the next
 * check that is not down; the newest stays open while the service is still down.
 */
export function outagesFromChecks(recent: WebServiceHealthPoint[]): Outage[] {
  const outages: Outage[] = [];
  let open: Outage | null = null;
  for (const point of recent) {
    const down = point.status.trim().toLowerCase() === "down";
    if (down && !open) {
      open = { start: point.at };
      outages.push(open);
    } else if (!down && open) {
      open.end = point.at;
      open = null;
    }
  }
  return outages.reverse();
}

/** Average and peak answer time over the checks that measured one (rounded ms), or nulls without any. */
export function latencyStats(recent: WebServiceHealthPoint[]): { avg: number | null; peak: number | null } {
  const times = recent.map((point) => point.response_ms ?? 0).filter((ms) => ms > 0);
  if (times.length === 0) return { avg: null, peak: null };
  return {
    avg: Math.round(times.reduce((sum, ms) => sum + ms, 0) / times.length),
    peak: Math.round(Math.max(...times)),
  };
}

/** A check time as a clock time today ("21:12"), with the day before that ("1 Nov, 21:12"), or "" when unreadable. */
export function formatCheckTime(iso: string | undefined, locale: string, nowMS = Date.now()): string {
  const at = iso ? new Date(iso) : null;
  if (!at || Number.isNaN(at.getTime())) return "";
  const today = new Date(nowMS).toDateString() === at.toDateString();
  return at.toLocaleString(locale, today
    ? { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }
    : { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
}

/** The hub's own error text, or the fallback when there is none or it is only a status code ("HTTP 500"). */
export function readableError(err: unknown, fallback: string): string {
  const message = err instanceof Error ? err.message.trim() : "";
  return !message || /^(HTTP\s*)?\d{3}$/i.test(message) || /\(HTTP \d{3}\)$/.test(message) ? fallback : message;
}

/** Shows a result to the person (the page passes the toast). */
export type Notify = (type: "success" | "error" | "info", message: string) => void;
