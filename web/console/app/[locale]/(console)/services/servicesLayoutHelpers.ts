import type { WebService } from "../../../hooks/useWebServices";

export const CATEGORY_ORDER = [
  "Media",
  "Downloads",
  "Gaming",
  "Networking",
  "Monitoring",
  "Home Automation",
  "Management",
  "Development",
  "Storage",
  "Databases",
  "Security",
  "Productivity",
  "Other",
];

export const SERVICE_LAYOUT_STORAGE_KEY = "labtether.services.layout.v1";

export type ServiceLayoutState = Record<string, string[]>;
export type ServiceHealthFilter = "all" | "unstable" | "changed_recently";
export type ServiceSortMode = "default" | "most_unstable" | "uptime_high" | "recently_changed";

export interface DockerImagePullPlanItem {
  hostAssetID: string;
  hostLabel: string;
  image: string;
}

export interface DockerImagePullPlan {
  items: DockerImagePullPlanItem[];
  missingImageCount: number;
}

export function formatResponseTime(ms: number): string {
  if (ms <= 0) return "";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(1)} s`;
}

export function proxyProviderLabel(provider: string): string {
  const normalized = provider.trim().toLowerCase();
  switch (normalized) {
    case "traefik":
      return "Traefik";
    case "caddy":
      return "Caddy";
    case "npm":
    case "nginx":
    case "nginx-proxy-manager":
      return "Nginx";
    default:
      return provider;
  }
}

export function compatConnectorLabel(connector: string): string {
  const normalized = connector.trim().toLowerCase();
  switch (normalized) {
    case "homeassistant":
    case "home-assistant":
      return "Home Assistant";
    case "proxmox":
      return "Proxmox VE";
    case "pbs":
      return "Proxmox Backup";
    case "truenas":
      return "TrueNAS";
    case "portainer":
      return "Portainer";
    case "docker":
      return "Docker API";
    default:
      return connector;
  }
}

export function formatCompatConfidence(raw?: string): string {
  if (!raw) return "";
  const parsed = Number(raw.trim());
  if (!Number.isFinite(parsed) || parsed <= 0) return "";
  const bounded = Math.max(0, Math.min(1, parsed));
  return `${Math.round(bounded * 100)}%`;
}

function serviceRank(service: WebService): number {
  if (service.metadata?.proxy_provider) {
    return 0;
  }
  if (service.source === "proxy") {
    return 1;
  }
  return 2;
}

export function defaultServiceCompare(a: WebService, b: WebService): number {
  const rankDiff = serviceRank(a) - serviceRank(b);
  if (rankDiff !== 0) return rankDiff;
  const nameDiff = a.name.localeCompare(b.name);
  if (nameDiff !== 0) return nameDiff;
  return a.url.localeCompare(b.url);
}

const serviceUnstableUptimeThresholdPercent = 95;
const serviceRecentStatusChangeWindowMS = 2 * 60 * 60 * 1000;

function parseStatusChangeTime(service: WebService): number | null {
  const raw = service.health?.last_change_at;
  if (!raw) {
    return null;
  }
  const parsed = Date.parse(raw);
  if (!Number.isFinite(parsed)) {
    return null;
  }
  return parsed;
}

function serviceUptimePercent(service: WebService): number | null {
  const value = service.health?.uptime_percent;
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return null;
  }
  return value;
}

function compareOptionalNumberAscending(left: number | null, right: number | null): number {
  if (left == null && right == null) return 0;
  if (left == null) return 1;
  if (right == null) return -1;
  return left - right;
}

function compareOptionalNumberDescending(left: number | null, right: number | null): number {
  if (left == null && right == null) return 0;
  if (left == null) return 1;
  if (right == null) return -1;
  return right - left;
}

export function matchesServiceHealthFilter(
  service: WebService,
  filter: ServiceHealthFilter,
  nowMS = Date.now(),
): boolean {
  if (filter === "all") {
    return true;
  }
  if (filter === "unstable") {
    const uptime = serviceUptimePercent(service);
    return uptime != null && uptime < serviceUnstableUptimeThresholdPercent;
  }
  const changedAt = parseStatusChangeTime(service);
  if (changedAt == null) {
    return false;
  }
  return nowMS - changedAt <= serviceRecentStatusChangeWindowMS;
}

export function serviceSortCompare(
  left: WebService,
  right: WebService,
  sortMode: ServiceSortMode,
): number {
  if (sortMode === "default") {
    return defaultServiceCompare(left, right);
  }

  const leftDown = left.status === "down";
  const rightDown = right.status === "down";
  const leftUptime = serviceUptimePercent(left);
  const rightUptime = serviceUptimePercent(right);
  const leftChangedAt = parseStatusChangeTime(left);
  const rightChangedAt = parseStatusChangeTime(right);

  if (sortMode === "most_unstable") {
    if (leftDown !== rightDown) {
      return leftDown ? -1 : 1;
    }
    const uptimeDiff = compareOptionalNumberAscending(leftUptime, rightUptime);
    if (uptimeDiff !== 0) {
      return uptimeDiff;
    }
    const changedDiff = compareOptionalNumberDescending(leftChangedAt, rightChangedAt);
    if (changedDiff !== 0) {
      return changedDiff;
    }
    return defaultServiceCompare(left, right);
  }

  if (sortMode === "uptime_high") {
    const uptimeDiff = compareOptionalNumberDescending(leftUptime, rightUptime);
    if (uptimeDiff !== 0) {
      return uptimeDiff;
    }
    return defaultServiceCompare(left, right);
  }

  const changedDiff = compareOptionalNumberDescending(leftChangedAt, rightChangedAt);
  if (changedDiff !== 0) {
    return changedDiff;
  }
  if (leftDown !== rightDown) {
    return leftDown ? -1 : 1;
  }
  const uptimeDiff = compareOptionalNumberAscending(leftUptime, rightUptime);
  if (uptimeDiff !== 0) {
    return uptimeDiff;
  }
  return defaultServiceCompare(left, right);
}

export function serviceLayoutKey(service: WebService): string {
  return `${service.host_asset_id}::${service.id}`;
}

export function serviceOverrideLookupKey(hostAssetID: string, serviceID: string): string {
  return `${hostAssetID}::${serviceID}`;
}

export function moveArrayItem<T>(items: T[], fromIndex: number, toIndex: number): T[] {
  if (
    fromIndex < 0 ||
    toIndex < 0 ||
    fromIndex >= items.length ||
    toIndex >= items.length ||
    fromIndex === toIndex
  ) {
    return items;
  }
  const next = [...items];
  const [item] = next.splice(fromIndex, 1);
  next.splice(toIndex, 0, item);
  return next;
}

export async function runWithConcurrencyLimit(
  tasks: Array<() => Promise<void>>,
  concurrency: number
): Promise<void> {
  if (tasks.length === 0) {
    return;
  }
  const workerCount = Math.max(1, Math.min(concurrency, tasks.length));
  let nextIndex = 0;
  await Promise.all(
    Array.from({ length: workerCount }, async () => {
      while (nextIndex < tasks.length) {
        const taskIndex = nextIndex;
        nextIndex += 1;
        await tasks[taskIndex]();
      }
    })
  );
}

export function buildCategoryLayoutOrder(
  category: string,
  services: WebService[],
  existingOrder: string[]
): string[] {
  const inCategory = services
    .filter((service) => service.category === category)
    .sort(defaultServiceCompare);
  const knownKeys = inCategory.map((service) => serviceLayoutKey(service));
  const knownSet = new Set(knownKeys);

  const normalizedExisting = existingOrder.filter((key) => knownSet.has(key));
  const existingSet = new Set(normalizedExisting);
  for (const key of knownKeys) {
    if (!existingSet.has(key)) {
      normalizedExisting.push(key);
    }
  }
  return normalizedExisting;
}

export function buildDockerImagePullPlan(
  services: WebService[],
  hostFilter: string,
  assetNameMap: Map<string, string>
): DockerImagePullPlan {
  const seen = new Set<string>();
  const items: DockerImagePullPlanItem[] = [];
  let missingImageCount = 0;

  for (const service of services) {
    if (service.source !== "docker") {
      continue;
    }
    if (hostFilter !== "all" && service.host_asset_id !== hostFilter) {
      continue;
    }

    const image = service.metadata?.image?.trim();
    if (!image) {
      missingImageCount += 1;
      continue;
    }

    const dedupeKey = `${service.host_asset_id.toLowerCase()}::${image.toLowerCase()}`;
    if (seen.has(dedupeKey)) {
      continue;
    }
    seen.add(dedupeKey);
    items.push({
      hostAssetID: service.host_asset_id,
      hostLabel: assetNameMap.get(service.host_asset_id) ?? service.host_asset_id,
      image,
    });
  }

  return {
    items,
    missingImageCount,
  };
}
