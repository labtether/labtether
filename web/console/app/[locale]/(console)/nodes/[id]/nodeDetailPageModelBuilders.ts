"use client";

import { assetFreshnessLabel } from "../../../../console/formatters";
import {
  CATEGORIES,
  assetCategory,
  childParentKey,
  groupByCategory,
  hostParentKey,
  isInfraHost,
} from "../../../../console/taxonomy";
import type { CategoryDef, CategorySlug } from "../../../../console/taxonomy";
import type { Asset } from "../../../../console/models";
import type { PanelContext, ConnectionBadge } from "./devicePanels";

const infraPanelSources = new Set(["proxmox", "pbs", "truenas", "docker", "portainer", "homeassistant"]);

export function deriveFreshnessClass(asset: Asset | null): "ok" | "pending" | "bad" {
  const freshness = asset ? assetFreshnessLabel(asset.last_seen_at) : "unknown";
  if (freshness === "online") {
    return "ok";
  }
  if (freshness === "unresponsive") {
    return "pending";
  }
  return "bad";
}

export function deriveEffectiveKind(asset: Asset | null, proxmoxKind: string): string {
  return proxmoxKind
    || (asset?.type === "hypervisor-node" ? "node" : "")
    || (asset?.type === "vm" ? "qemu" : "")
    || (asset?.type === "container" ? "lxc" : "")
    || (asset?.type === "storage-pool" ? "storage" : "");
}

export function deriveProxmoxTarget(proxmoxNode: string, proxmoxVMID: string): string {
  return proxmoxNode && proxmoxVMID ? `${proxmoxNode}/${proxmoxVMID}` : "";
}

export function isInfraAsset(asset: Asset | null): boolean {
  return asset ? infraPanelSources.has(asset.source) && isInfraHost(asset) : false;
}

export function deriveMergedDockerHost(asset: Asset | null, assets: Asset[] | undefined): Asset | null {
  if (!asset || asset.source !== "agent" || !assets) {
    return null;
  }
  return assets.find(
    (candidate) =>
      candidate.source === "docker"
      && candidate.type === "container-host"
      && candidate.metadata?.agent_id === asset.id
  ) ?? null;
}

export function deriveParentInfraHost(asset: Asset | null, assets: Asset[] | undefined): Asset | null {
  if (!asset || !assets || isInfraHost(asset)) {
    return null;
  }

  const parentKey = childParentKey(asset);
  if (!parentKey) {
    return null;
  }

  return assets.find((candidate) => {
    if (candidate.source !== asset.source) {
      return false;
    }
    if (!isInfraHost(candidate)) {
      return false;
    }
    return hostParentKey(candidate) === parentKey;
  }) ?? null;
}

export function deriveInfraCategories(
  asset: Asset | null,
  assets: Asset[] | undefined,
  isInfra: boolean
): Map<CategorySlug, { def: CategoryDef; count: number }> {
  if (!isInfra || !asset || !assets) {
    return new Map<CategorySlug, { def: CategoryDef; count: number }>();
  }

  const hostKey = hostParentKey(asset);
  const childAssets = assets.filter(
    (candidate) =>
      candidate.id !== asset.id
      && candidate.source === asset.source
      && childParentKey(candidate) === hostKey
      && assetCategory(candidate.type) !== undefined
  );
  const grouped = groupByCategory(childAssets);
  const result = new Map<CategorySlug, { def: CategoryDef; count: number }>();
  for (const category of CATEGORIES) {
    for (const [def, groupedAssets] of grouped.entries()) {
      if (def.slug === category.slug && groupedAssets.length > 0) {
        result.set(category.slug, { def, count: groupedAssets.length });
      }
    }
  }
  return result;
}

/** Asset types that can plausibly have a remote desktop/viewer. */
const DESKTOP_ELIGIBLE_TYPES = new Set(["host", "hypervisor-node", "vm", "container"]);

export function deriveShouldShowDesktop({
  assetType,
  isDockerContainer,
  isInfra,
  isDockerHost,
  isProxmoxAsset,
  effectiveKind,
}: {
  assetType: string;
  isDockerContainer: boolean;
  isInfra: boolean;
  isDockerHost: boolean;
  isProxmoxAsset: boolean;
  effectiveKind: string;
}): boolean {
  if (isDockerContainer) {
    return false;
  }
  if (isInfra) {
    if (isDockerHost) {
      return false;
    }
    return isProxmoxAsset && effectiveKind === "node";
  }
  // Only standalone hosts and Proxmox guests (qemu/lxc) can show desktop.
  // This prevents storage-pool, dataset, disk, share-*, service, app,
  // compose-stack, ha-entity, etc. from showing a Remote View panel.
  if (!DESKTOP_ELIGIBLE_TYPES.has(assetType)) {
    return false;
  }
  return !isProxmoxAsset || effectiveKind === "qemu" || effectiveKind === "lxc";
}

export function buildNodePanelContext({
  asset,
  nodeHasAgent,
  mergedDockerHost,
  infraCategories,
  isProxmoxAsset,
  isTrueNASAsset,
  isPBSAsset,
  isInfra,
  isLinuxAgentNode,
  supportsServiceListing,
  supportsPackageListing,
  supportsScheduleListing,
  supportsNetworkListing,
  shouldShowDesktop,
  protocolCount,
  webServiceCount,
  connectionBadges,
}: {
  asset: Asset;
  nodeHasAgent: boolean;
  mergedDockerHost: Asset | null;
  infraCategories: Map<CategorySlug, { def: CategoryDef; count: number }>;
  isProxmoxAsset: boolean;
  isTrueNASAsset: boolean;
  isPBSAsset: boolean;
  isInfra: boolean;
  isLinuxAgentNode: boolean;
  supportsServiceListing: boolean;
  supportsPackageListing: boolean;
  supportsScheduleListing: boolean;
  supportsNetworkListing: boolean;
  shouldShowDesktop: boolean;
  protocolCount: number;
  webServiceCount: number;
  connectionBadges: ConnectionBadge[];
}): PanelContext {
  const isHomeAssistantHub = asset.source === "homeassistant" && asset.type === "connector-cluster";
  const isHomeAssistantEntity = asset.source === "homeassistant" && asset.type === "ha-entity";

  return {
    asset,
    nodeHasAgent,
    mergedDockerHost,
    metadata: asset.metadata ?? {},
    infraCategories: new Map(
      Array.from(infraCategories.entries()).map(([slug, { count }]) => [slug, { count }])
    ),
    isProxmoxAsset,
    isTrueNASAsset,
    isPBSAsset,
    isHomeAssistantAsset: isHomeAssistantHub || isHomeAssistantEntity,
    isHomeAssistantHub,
    isHomeAssistantEntity,
    isInfra,
    isLinuxAgentNode,
    supportsServiceListing,
    supportsPackageListing,
    supportsScheduleListing,
    supportsNetworkListing,
    desktopEligible: shouldShowDesktop,
    protocolCount,
    webServiceCount,
    connectionBadges,
  };
}

export function deriveDockerHostForPanel({
  isDockerHost,
  dockerHostId,
  mergedDockerHostId,
  nodeId,
}: {
  isDockerHost: boolean;
  dockerHostId: string;
  mergedDockerHostId: string;
  nodeId: string;
}): string {
  if (isDockerHost) {
    return dockerHostId;
  }
  if (mergedDockerHostId) {
    return mergedDockerHostId;
  }
  return nodeId;
}
