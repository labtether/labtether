import { assetFreshnessLabel, formatAge } from "../../../../console/formatters";
import type { Asset } from "../../../../console/models";
import { assetCategory, childParentKey, hostParentKey } from "../../../../console/taxonomy";
import {
  confidenceLabel,
  isHealthyHealth,
  normalizeHealth,
  normalizeRiskState,
  parseIDList,
  parsePercent,
  parseStorageName,
  riskStateFromScore,
  storageKey,
} from "./storageOperationsUtils";
import type { ProxmoxZFSPool, StorageInsightsResponse, StorageRow } from "./storageOperationsTypes";

const TELEMETRY_STALE_MS = 3 * 60_000;

export function deriveStorageAssets(allAssets: Asset[], hostAsset: Asset): Asset[] {
  const hostKey = hostParentKey(hostAsset);
  return allAssets.filter((asset) => {
    if (asset.id === hostAsset.id) return false;
    if (asset.source !== hostAsset.source) return false;
    if (childParentKey(asset) !== hostKey) return false;
    return assetCategory(asset.type) === "storage";
  });
}

export function deriveProxmoxStaleInfo(fetchedAtRaw: string | undefined): { stale: boolean; label: string } {
  const fetchedAt = fetchedAtRaw?.trim() ?? "";
  if (fetchedAt === "") {
    return { stale: true, label: "never" };
  }
  const parsed = new Date(fetchedAt);
  if (!Number.isFinite(parsed.getTime())) {
    return { stale: true, label: "invalid timestamp" };
  }
  const ageMs = Date.now() - parsed.getTime();
  return {
    stale: !Number.isFinite(ageMs) || ageMs > TELEMETRY_STALE_MS,
    label: formatAge(fetchedAt),
  };
}

export function buildFallbackRows(
  storageAssets: Asset[],
  zfsPools: ProxmoxZFSPool[],
  proxmoxStaleInfo: { stale: boolean; label: string },
): StorageRow[] {
  const zfsByName = new Map<string, ProxmoxZFSPool>();
  for (const pool of zfsPools) {
    const poolName = pool.name?.trim() ?? "";
    if (poolName !== "") {
      zfsByName.set(storageKey(poolName), pool);
    }
  }

  const rowMap = new Map<string, StorageRow>();

  for (const asset of storageAssets) {
    const poolName = parseStorageName(asset);
    const key = storageKey(poolName);
    const zfs = zfsByName.get(key);

    const zfsHealth = normalizeHealth(zfs?.health);
    const metadataHealth = normalizeHealth(asset.metadata?.status);
    const health = zfs ? zfsHealth : metadataHealth;

    const zfsUsedPercent = zfs?.size && zfs.size > 0 && zfs.alloc != null
      ? (zfs.alloc / zfs.size) * 100
      : null;
    const usedPercent = zfsUsedPercent != null ? Math.min(Math.max(zfsUsedPercent, 0), 100) : parsePercent(asset.metadata?.disk_percent);

    const staleLabel = formatAge(asset.last_seen_at);
    const stale = assetFreshnessLabel(asset.last_seen_at) !== "online";
    const isHealthy = isHealthyHealth(health);

    let riskScore = 0;
    let reason = "Healthy baseline.";
    if (!isHealthy) {
      riskScore += 70;
      reason = `Pool health is ${health}.`;
    }
    if (usedPercent != null && usedPercent >= 90) {
      riskScore += 35;
      if (isHealthy) reason = "Capacity is above 90%.";
    } else if (usedPercent != null && usedPercent >= 80) {
      riskScore += 20;
      if (isHealthy) reason = "Capacity is above 80%.";
    }
    if (stale) {
      riskScore += 20;
      if (isHealthy && usedPercent == null) reason = "Telemetry is stale.";
    }
    if (usedPercent == null) {
      riskScore += 5;
    }

    const typeLabel = asset.metadata?.plugintype?.trim() || (zfs ? "zfspool" : "storage");

    rowMap.set(key, {
      key,
      poolName,
      typeLabel,
      health,
      usedPercent,
      freeBytes: zfs?.free ?? null,
      allocBytes: zfs?.alloc ?? null,
      fragPercent: zfs?.frag ?? null,
      dedupRatio: zfs?.dedup ?? null,
      growthBytes7d: null,
      daysTo80: null,
      daysToFull: null,
      confidence: "low",
      snapshotCount: 0,
      snapshotBytes: 0,
      vmCount: 0,
      ctCount: 0,
      vmIDs: [],
      ctIDs: [],
      stale,
      staleLabel,
      scrubOverdue: false,
      riskScore,
      riskState: riskStateFromScore(riskScore),
      reason,
    });
  }

  for (const pool of zfsPools) {
    const poolName = pool.name?.trim() ?? "";
    if (poolName === "") continue;
    const key = storageKey(poolName);
    if (rowMap.has(key)) continue;

    const health = normalizeHealth(pool.health);
    const usedPercent = pool.size && pool.size > 0 && pool.alloc != null
      ? Math.min(Math.max((pool.alloc / pool.size) * 100, 0), 100)
      : null;

    let riskScore = 0;
    let reason = "Healthy baseline.";
    if (!isHealthyHealth(health)) {
      riskScore += 70;
      reason = `Pool health is ${health}.`;
    }
    if (usedPercent != null && usedPercent >= 90) {
      riskScore += 35;
      if (isHealthyHealth(health)) reason = "Capacity is above 90%.";
    } else if (usedPercent != null && usedPercent >= 80) {
      riskScore += 20;
      if (isHealthyHealth(health)) reason = "Capacity is above 80%.";
    }
    if (proxmoxStaleInfo.stale) {
      riskScore += 20;
      if (usedPercent == null) reason = "Telemetry is stale.";
    }
    if (usedPercent == null) {
      riskScore += 5;
    }

    rowMap.set(key, {
      key,
      poolName,
      typeLabel: "zfspool",
      health,
      usedPercent,
      freeBytes: pool.free ?? null,
      allocBytes: pool.alloc ?? null,
      fragPercent: pool.frag ?? null,
      dedupRatio: pool.dedup ?? null,
      growthBytes7d: null,
      daysTo80: null,
      daysToFull: null,
      confidence: "low",
      snapshotCount: 0,
      snapshotBytes: 0,
      vmCount: 0,
      ctCount: 0,
      vmIDs: [],
      ctIDs: [],
      stale: proxmoxStaleInfo.stale,
      staleLabel: proxmoxStaleInfo.label,
      scrubOverdue: false,
      riskScore,
      riskState: riskStateFromScore(riskScore),
      reason,
    });
  }

  return Array.from(rowMap.values()).sort((left, right) => {
    if (left.riskScore !== right.riskScore) return right.riskScore - left.riskScore;
    const leftUsed = left.usedPercent ?? -1;
    const rightUsed = right.usedPercent ?? -1;
    if (leftUsed !== rightUsed) return rightUsed - leftUsed;
    return left.poolName.localeCompare(right.poolName);
  });
}

export function buildInsightRows(insights: StorageInsightsResponse | null): StorageRow[] {
  const pools = insights?.pools ?? [];
  if (pools.length === 0) return [];

  return pools.map((pool, idx) => {
    const riskScore = Number.isFinite(pool.risk_score) ? Number(pool.risk_score) : 0;
    const riskState = normalizeRiskState(pool.risk_state, riskScore);
    const reason = pool.reasons?.find((entry) => entry.trim() !== "") ?? "No immediate storage risk detected.";
    const confidence = confidenceLabel(pool.forecast?.confidence ?? "low");

    return {
      key: storageKey(pool.name || `pool-${idx}`),
      poolName: pool.name || `pool-${idx}`,
      typeLabel: pool.dedup_ratio != null || pool.frag_percent != null ? "zfspool" : "storage",
      health: normalizeHealth(pool.health),
      usedPercent: pool.used_percent != null ? Number(pool.used_percent) : null,
      freeBytes: pool.free_bytes != null ? Number(pool.free_bytes) : null,
      allocBytes: pool.used_bytes != null ? Number(pool.used_bytes) : null,
      fragPercent: pool.frag_percent != null ? Number(pool.frag_percent) : null,
      dedupRatio: pool.dedup_ratio != null ? Number(pool.dedup_ratio) : null,
      growthBytes7d: pool.growth_bytes_7d != null ? Number(pool.growth_bytes_7d) : null,
      daysTo80: pool.forecast?.days_to_80 != null ? Number(pool.forecast.days_to_80) : null,
      daysToFull: pool.forecast?.days_to_full != null ? Number(pool.forecast.days_to_full) : null,
      confidence,
      snapshotCount: Number(pool.snapshots?.count ?? 0),
      snapshotBytes: Number(pool.snapshots?.bytes ?? 0),
      vmCount: Number(pool.dependent_workloads?.vm_count ?? 0),
      ctCount: Number(pool.dependent_workloads?.ct_count ?? 0),
      vmIDs: parseIDList(pool.dependent_workloads?.vm_ids),
      ctIDs: parseIDList(pool.dependent_workloads?.ct_ids),
      stale: !!pool.telemetry_stale,
      staleLabel: pool.telemetry_stale ? "stale" : "fresh",
      scrubOverdue: !!pool.scrub?.overdue,
      riskScore,
      riskState,
      reason,
    };
  });
}
