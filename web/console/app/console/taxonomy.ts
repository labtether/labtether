import type { Asset } from "./models";
import { formatMetadataLabel, assetFreshnessLabel } from "./formatters";
import type { LucideIcon } from "lucide-react";
import {
  AppWindow,
  Box,
  Container,
  Cpu,
  Database,
  HardDrive,
  Home,
  Layers,
  Monitor,
  Package,
  Server,
  Workflow,
} from "lucide-react";

// --- Canonical categories (ordered by display priority) ---

export type CategorySlug = "compute" | "storage" | "network" | "services" | "power";

export type CategoryDef = {
  slug: CategorySlug;
  label: string;
};

export const CATEGORIES: CategoryDef[] = [
  { slug: "compute",  label: "Compute" },
  { slug: "storage",  label: "Storage" },
  { slug: "network",  label: "Network" },
  { slug: "services", label: "Services" },
  { slug: "power",    label: "Power" },
];

// --- Asset type → category mapping ---

const ASSET_CATEGORY: Record<string, CategorySlug> = {
  // Compute
  node: "compute",
  host: "compute",
  "hypervisor-node": "compute",
  nas: "compute",
  "storage-controller": "compute",
  vm: "compute",
  container: "compute",
  app: "compute",
  pod: "compute",
  deployment: "compute",
  "container-host": "compute",
  "docker-container": "compute",
  // Storage
  "storage-pool": "storage",
  datastore: "storage",
  dataset: "storage",
  disk: "storage",
  "share-smb": "storage",
  "share-nfs": "storage",
  // Network (future connector types will be added here)
  // Services
  service: "services",
  "ha-entity": "services",
  "compose-stack": "services",
  stack: "services",
  // Power (future connector types will be added here)
};

export function assetCategory(type: string): CategorySlug | undefined {
  return ASSET_CATEGORY[type];
}

// --- Visibility tiers ---

export type VisibilityTier = "device" | "workload" | "resource";

const VISIBILITY_TIER: Record<string, VisibilityTier> = {
  // Devices — always visible
  node: "device",
  host: "device",
  "hypervisor-node": "device",
  nas: "device",
  "storage-controller": "device",
  vm: "device",
  "container-host": "device",
  // Workloads — hidden when healthy
  container: "workload",
  "docker-container": "workload",
  app: "workload",
  pod: "workload",
  deployment: "workload",
  stack: "workload",
  "compose-stack": "workload",
  service: "workload",
  "backup-task": "workload",
  "replication-task": "workload",
  "cloud-sync-task": "workload",
  "cron-task": "workload",
  // Resources — hidden when healthy
  "storage-pool": "resource",
  datastore: "resource",
  dataset: "resource",
  disk: "resource",
  volume: "resource",
  "share-smb": "resource",
  "share-nfs": "resource",
  snapshot: "resource",
  interface: "resource",
  vlan: "resource",
  "firewall-rule": "resource",
};

export function getVisibilityTier(asset: { type: string; source: string }): VisibilityTier | null {
  if (asset.source === "homeassistant") return null;
  return VISIBILITY_TIER[asset.type] ?? null;
}

const UNHEALTHY_STATUSES = new Set([
  "offline", "down", "critical", "error",
  "unhealthy", "warning",
  "stopped", "exited", "dead",
  "degraded", "restarting",
  "stale", "unresponsive",
  "failed",
]);

export function isAssetHealthy(asset: { status: string; last_seen_at: string }): boolean {
  const raw = asset.status.trim().toLowerCase();
  if (UNHEALTHY_STATUSES.has(raw)) return false;
  const freshness = assetFreshnessLabel(asset.last_seen_at);
  return freshness === "online";
}

export function isVisibleOnDashboard(asset: { type: string; source: string; status: string; last_seen_at: string }): boolean {
  const tier = getVisibilityTier(asset);
  if (tier === null) return false;
  if (tier === "device") return true;
  return !isAssetHealthy(asset);
}

export function isDeviceTier(asset: { type: string; source: string }): boolean {
  return getVisibilityTier(asset) === "device";
}

export function isHomeAssistantHubAsset(asset: { type: string; source: string }): boolean {
  return asset.source === "homeassistant" && asset.type === "connector-cluster";
}

export function isHomeAssistantEntityAsset(asset: { type: string; source: string }): boolean {
  return asset.source === "homeassistant" && asset.type === "ha-entity";
}

// --- Infrastructure host registry ---

type InfraHostDef = {
  hostType: string;
};

const INFRA_HOSTS: Record<string, InfraHostDef> = {
  proxmox: { hostType: "hypervisor-node" },
  pbs: { hostType: "storage-controller" },
  truenas: { hostType: "nas" },
  docker: { hostType: "container-host" },
  portainer: { hostType: "container-host" },
  homeassistant: { hostType: "connector-cluster" },
  // Future:
  // unraid:  { hostType: "nas" },
};
const INFRA_HOST_TYPES = new Set(["node", "host", "hypervisor-node", "nas", "storage-controller", "container-host"]);
const COLLECTOR_SCOPED_INFRA_SOURCES = new Set(["pbs", "truenas", "homeassistant"]);

const HIDDEN_TYPES = new Set(["connector-cluster"]);

// --- Helper functions ---

export function isInfraHost(asset: Asset): boolean {
  const sourceHostType = INFRA_HOSTS[asset.source]?.hostType;
  if (sourceHostType === asset.type) return true;
  // Fallback to canonical host types for sources that do not publish hostType mappings
  // (for example agent-managed "host" assets used in topology/device hierarchy views).
  return INFRA_HOST_TYPES.has(asset.type);
}

export function isInfraChild(asset: Asset): boolean {
  return asset.source in INFRA_HOSTS
    && !isInfraHost(asset)
    && !HIDDEN_TYPES.has(asset.type);
}

export function isHiddenAsset(asset: Asset): boolean {
  return HIDDEN_TYPES.has(asset.type);
}

/**
 * Returns a key used to match children to their infrastructure parent.
 * Proxmox children carry metadata.node matching the server name.
 * Portainer children carry metadata.endpoint_id, and Docker children/hosts
 * carry metadata.agent_id when available (with docker ID-pattern fallback for
 * older rows where metadata can be incomplete). Collector-scoped connectors
 * such as PBS, TrueNAS, and Home Assistant prefer metadata.collector_id so
 * multiple configured connectors do not bleed into one hierarchy. Remaining
 * sources use the source name as key (one host per source).
 */
function normalizeDockerParentKey(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  if (!trimmed) return undefined;
  return trimmed
    .toLowerCase()
    .replace(/\s+/g, "-")
    .replaceAll(".", "-");
}

function dockerParentKeyFromAssetID(assetID: string): string | undefined {
  const normalizedID = normalizeDockerParentKey(assetID);
  if (!normalizedID) return undefined;

  if (normalizedID === "docker") return "docker";
  if (normalizedID.startsWith("docker-host-")) {
    return normalizedID.slice("docker-host-".length) || undefined;
  }

  if (normalizedID.startsWith("docker-ct-")) {
    const suffix = normalizedID.slice("docker-ct-".length);
    const split = suffix.lastIndexOf("-");
    if (split > 0) return suffix.slice(0, split);
  }

  if (normalizedID.startsWith("docker-stack-")) {
    const suffix = normalizedID.slice("docker-stack-".length);
    const split = suffix.lastIndexOf("-");
    if (split > 0) return suffix.slice(0, split);
  }

  return undefined;
}

function dockerParentKeyFromHostName(name: string): string | undefined {
  const normalizedName = normalizeDockerParentKey(name);
  if (!normalizedName?.startsWith("docker-")) return undefined;
  return normalizedName.slice("docker-".length) || undefined;
}

function collectorScopedParentKey(asset: Asset): string | undefined {
  const collectorID = asset.metadata?.collector_id?.trim();
  return collectorID ? collectorID : undefined;
}

export function childParentKey(child: Asset): string | undefined {
  if (child.source === "proxmox") return child.metadata?.node;
  if (child.source === "portainer") return child.metadata?.endpoint_id;
  if (child.source === "docker") {
    return normalizeDockerParentKey(
      child.metadata?.agent_id
      ?? child.metadata?.host_id
      ?? dockerParentKeyFromAssetID(child.id),
    ) ?? child.source;
  }
  if (COLLECTOR_SCOPED_INFRA_SOURCES.has(child.source)) {
    return collectorScopedParentKey(child) ?? child.source;
  }
  if (child.source in INFRA_HOSTS) return child.source;
  return undefined;
}

export function hostParentKey(server: Asset): string {
  if (server.source === "proxmox") return server.name;
  if (server.source === "portainer") return server.metadata?.endpoint_id ?? server.name;
  if (server.source === "docker") {
    return normalizeDockerParentKey(
      server.metadata?.agent_id
      ?? server.metadata?.host_id
      ?? dockerParentKeyFromAssetID(server.id)
      ?? dockerParentKeyFromHostName(server.name),
    ) ?? server.source;
  }
  if (COLLECTOR_SCOPED_INFRA_SOURCES.has(server.source)) {
    return collectorScopedParentKey(server) ?? server.source;
  }
  return server.source;
}

/**
 * Group an array of child assets by category slug.
 * Returns a Map preserving CATEGORIES display order, skipping empty categories.
 */
export function groupByCategory(assets: Asset[]): Map<CategoryDef, Asset[]> {
  const buckets = new Map<CategorySlug, Asset[]>();
  for (const asset of assets) {
    const slug = assetCategory(asset.type);
    if (!slug) continue;
    const list = buckets.get(slug) ?? [];
    list.push(asset);
    buckets.set(slug, list);
  }
  const result = new Map<CategoryDef, Asset[]>();
  for (const cat of CATEGORIES) {
    const list = buckets.get(cat.slug);
    if (list && list.length > 0) {
      result.set(cat, list);
    }
  }
  return result;
}

/**
 * Sort assets within a category: by type alphabetically, then by name.
 * This groups storage-pools before datasets before disks naturally.
 */
export function sortCategoryAssets(assets: Asset[]): Asset[] {
  return [...assets].sort((a, b) => {
    const typeCmp = a.type.localeCompare(b.type);
    if (typeCmp !== 0) return typeCmp;
    return a.name.localeCompare(b.name);
  });
}

// --- Friendly type labels ---

const FRIENDLY_TYPE_LABELS: Record<string, string> = {
  node: "Node",
  host: "Host",
  "hypervisor-node": "Hypervisor Node",
  nas: "NAS",
  "storage-controller": "Storage Controller",
  vm: "Virtual Machine",
  "container-host": "Container Host",
  container: "Container",
  stack: "Stack",
  "storage-pool": "Storage Pool",
  datastore: "Datastore",
  dataset: "Dataset",
  disk: "Disk",
  "share-smb": "SMB Share",
  "share-nfs": "NFS Share",
  service: "Service",
  "ha-entity": "HA Entity",
  app: "Application",
  pod: "Pod",
  deployment: "Deployment",
  "connector-cluster": "Connector Cluster",
  agent: "Agent",
  "docker-container": "Docker Container",
  "compose-stack": "Compose Stack",
};

export function friendlyTypeLabel(type: string): string {
  return FRIENDLY_TYPE_LABELS[type] ?? formatMetadataLabel(type);
}

// --- Asset type icon mapping ---

const TYPE_ICONS: Record<string, LucideIcon> = {
  node: Server,
  host: Server,
  "hypervisor-node": Server,
  nas: Database,
  "storage-controller": Database,
  vm: Server,
  "container-host": Server,
  container: Container,
  "docker-container": Container,
  pod: Container,
  deployment: Layers,
  app: AppWindow,
  stack: Layers,
  "compose-stack": Layers,
  service: Workflow,
  "ha-entity": Home,
  "storage-pool": HardDrive,
  datastore: HardDrive,
  dataset: HardDrive,
  disk: HardDrive,
  "share-smb": HardDrive,
  "share-nfs": HardDrive,
  agent: Monitor,
  "connector-cluster": Package,
};

export function assetTypeIcon(type: string): LucideIcon {
  const normalized = type.trim().toLowerCase();
  const mapped = TYPE_ICONS[normalized];
  if (mapped) return mapped;

  if (
    normalized.includes("storage")
    || normalized.includes("disk")
    || normalized.includes("dataset")
    || normalized.includes("share")
  ) {
    return HardDrive;
  }
  if (normalized.includes("container") || normalized.includes("pod")) {
    return Container;
  }
  if (
    normalized.includes("service")
    || normalized.includes("stack")
    || normalized.includes("entity")
    || normalized.includes("deploy")
  ) {
    return Workflow;
  }
  if (
    normalized.includes("host")
    || normalized.includes("node")
    || normalized.includes("vm")
    || normalized === "nas"
  ) {
    return Server;
  }
  return Cpu;
}

// --- Source icon mapping ---

const SOURCE_ICONS: Record<string, LucideIcon> = {
  proxmox: Server,
  pbs: Database,
  truenas: Database,
  docker: Box,
  agent: Monitor,
  portainer: Container,
  homeassistant: Home,
};

export function sourceIcon(source: string): LucideIcon {
  const normalized = source.trim().toLowerCase();
  const mapped = SOURCE_ICONS[normalized];
  if (mapped) return mapped;

  if (normalized.includes("assistant")) return Home;
  if (normalized.includes("docker") || normalized.includes("container")) return Container;
  if (normalized.includes("nas") || normalized.includes("storage")) return Database;
  if (normalized.includes("backup") || normalized === "pbs") return Database;
  if (normalized.includes("proxmox") || normalized.includes("hypervisor")) return Server;
  if (normalized.includes("agent")) return Monitor;

  return Cpu;
}

const SOURCE_LABELS: Record<string, string> = {
  proxmox: "Proxmox",
  pbs: "Proxmox Backup Server",
  truenas: "TrueNAS",
  docker: "Docker",
  agent: "Agent",
  portainer: "Portainer",
  homeassistant: "Home Assistant",
};

export function friendlySourceLabel(source: string): string {
  return SOURCE_LABELS[source.toLowerCase()] ?? formatMetadataLabel(source);
}
