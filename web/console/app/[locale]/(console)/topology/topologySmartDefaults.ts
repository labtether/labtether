import type { RelationshipType } from "./topologyCanvasTypes";

// Asset type categories for smart defaults
const CONTAINER_TYPES = new Set(["container", "docker-container", "lxc", "pod"]);
const VM_TYPES = new Set(["vm", "virtual-machine"]);
const HOST_TYPES = new Set(["host", "server", "hypervisor", "container-host"]);
const STORAGE_TYPES = new Set(["nas", "storage", "datastore", "pool", "dataset"]);
const DB_TYPES = new Set(["database", "db"]);
const NETWORK_TYPES = new Set(["switch", "router", "firewall", "ap", "access-point"]);
const SERVICE_TYPES = new Set(["service", "ha-entity", "automation"]);

export function inferRelationshipType(
  sourceType: string,
  targetType: string,
): RelationshipType {
  const srcLower = sourceType.toLowerCase();
  const tgtLower = targetType.toLowerCase();

  // Container/VM -> Host = runs_on
  if ((CONTAINER_TYPES.has(srcLower) || VM_TYPES.has(srcLower)) && HOST_TYPES.has(tgtLower)) {
    return "runs_on";
  }

  // Service -> Database/Storage = depends_on
  if (SERVICE_TYPES.has(srcLower) && (DB_TYPES.has(tgtLower) || STORAGE_TYPES.has(tgtLower))) {
    return "depends_on";
  }

  // Service -> Service = depends_on
  if (SERVICE_TYPES.has(srcLower) && SERVICE_TYPES.has(tgtLower)) {
    return "depends_on";
  }

  // Host -> Network device = connected_to
  if (HOST_TYPES.has(srcLower) && NETWORK_TYPES.has(tgtLower)) {
    return "connected_to";
  }

  // Fallback
  return "connected_to";
}
