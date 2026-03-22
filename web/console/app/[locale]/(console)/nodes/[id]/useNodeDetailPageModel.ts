"use client";

import { useMemo } from "react";
import type {
  UseNodeDetailPageModelArgs,
  UseNodeDetailPageModelResult,
} from "./nodeDetailPageModelTypes";
import {
  deriveCanonicalDockerHostNodeID,
  deriveDockerHostId,
  deriveDockerStackHostId,
  deriveFallbackDockerHostNodeID,
  deriveMergedDockerHostId,
} from "./nodeDockerIdentityModel";
import { useNodeCapabilities } from "./useNodeCapabilities";
import { useProtocolConfigs } from "./useProtocolConfigs";
import { buildAvailablePanels } from "./devicePanels";
import {
  buildNodePanelContext,
  deriveDockerHostForPanel,
  deriveEffectiveKind,
  deriveFreshnessClass,
  deriveInfraCategories,
  deriveMergedDockerHost,
  deriveParentInfraHost,
  deriveProxmoxTarget,
  deriveShouldShowDesktop,
  isInfraAsset,
} from "./nodeDetailPageModelBuilders";
import { useWebServices } from "../../../../hooks/useWebServices";
import type { Asset } from "../../../../console/models";
import type { ConnectionBadge } from "./devicePanelTypes";

const PROTOCOL_LABELS_SHORT: Record<string, string> = {
  ssh: "SSH", telnet: "Telnet", vnc: "VNC", rdp: "RDP", ard: "ARD",
};

export function useNodeDetailPageModel({
  nodeId,
  assets,
  telemetry,
  groupLabelByID,
  nodeHasAgent,
}: UseNodeDetailPageModelArgs): UseNodeDetailPageModelResult {
  const placeholderAsset = useMemo<Asset>(() => ({
    id: nodeId,
    name: nodeId,
    type: "unknown",
    source: "unknown",
    status: "unknown",
    last_seen_at: "",
    metadata: {},
  }), [nodeId]);

  const asset = useMemo(() => {
    return assets?.find((candidate) => candidate.id === nodeId) ?? null;
  }, [nodeId, assets]);

  const telemetryOverview = useMemo(() => {
    return telemetry?.find((t) => t.asset_id === nodeId) ?? null;
  }, [nodeId, telemetry]);

  const freshnessClass = deriveFreshnessClass(asset);
  const groupName = asset?.group_id ? groupLabelByID.get(asset.group_id) ?? "Unknown" : "Unassigned";
  const proxmoxKind = (asset?.metadata?.proxmox_type ?? "").toLowerCase();
  const proxmoxNode = asset?.metadata?.node ?? "";
  const proxmoxVMID = asset?.metadata?.vmid ?? "";
  const isProxmoxAsset = asset?.source === "proxmox";
  const isTrueNASAsset = asset?.source === "truenas";
  const isPBSAsset = asset?.source === "pbs";
  const isDockerHost = asset?.source === "docker" && asset?.type === "container-host";
  const isDockerContainer = asset?.type === "docker-container" && asset?.source === "docker";
  const isDockerStack = asset?.type === "compose-stack" && asset?.source === "docker";
  const dockerHostId = useMemo(() => deriveDockerHostId(asset), [asset]);
  const dockerContainerId = isDockerContainer ? asset?.id ?? "" : "";
  const dockerStackName = isDockerStack ? asset?.name ?? "" : "";
  const dockerStackAssetId = isDockerStack ? asset?.id ?? "" : "";
  const dockerStackHostId = useMemo(
    () => deriveDockerStackHostId(asset, assets),
    [asset, assets]
  );
  const canonicalDockerHostNodeID = useMemo(
    () => deriveCanonicalDockerHostNodeID(asset),
    [asset]
  );
  const fallbackDockerHostNodeID = useMemo(
    () => deriveFallbackDockerHostNodeID(nodeId, assets, Boolean(asset)),
    [asset, nodeId, assets]
  );
  const effectiveKind = deriveEffectiveKind(asset, proxmoxKind);
  const proxmoxTarget = deriveProxmoxTarget(proxmoxNode, proxmoxVMID);

  const isInfra = isInfraAsset(asset);
  const metadata = asset?.metadata ?? {};
  const nodePlatform = (asset?.platform ?? metadata.platform ?? "").trim().toLowerCase();
  const {
    isLinuxAgentNode,
    supportsServiceListing,
    supportsPackageListing,
    supportsScheduleListing,
    supportsNetworkListing,
    supportsNetworkActions,
    supportsLogQuery,
    logQueryModeLabel,
    nodeNetworkMethodOptions,
    nodeNetworkControlsLabel,
    nodeNetworkControlsHint,
  } = useNodeCapabilities({
    metadata,
    nodeHasAgent,
    nodePlatform,
  });

  const { protocols } = useProtocolConfigs(nodeId);
  const { services: webServicesForNode } = useWebServices({ host: nodeId, detailLevel: "compact" });

  const mergedDockerHost = useMemo(() => {
    return deriveMergedDockerHost(asset, assets);
  }, [asset, assets]);

  const mergedDockerHostId = useMemo(
    () => deriveMergedDockerHostId(mergedDockerHost),
    [mergedDockerHost]
  );

  const parentInfraHost = useMemo(() => {
    return deriveParentInfraHost(asset, assets);
  }, [asset, assets]);

  const infraCategories = useMemo(() => {
    return deriveInfraCategories(asset, assets, isInfra);
  }, [isInfra, asset, assets]);

  const shouldShowDesktop = useMemo(() => {
    return deriveShouldShowDesktop({
      assetType: asset?.type ?? "",
      isDockerContainer,
      isInfra,
      isDockerHost,
      isProxmoxAsset,
      effectiveKind,
    });
  }, [asset?.type, effectiveKind, isDockerContainer, isDockerHost, isInfra, isProxmoxAsset]);

  const connectionBadges: ConnectionBadge[] = useMemo(() => [
    ...protocols.map(p => ({
      label: `${PROTOCOL_LABELS_SHORT[p.protocol] ?? p.protocol} :${p.port}`,
      status: p.test_status === "success" ? "ok" as const : p.test_status === "failed" ? "bad" as const : "unknown" as const,
    })),
    ...webServicesForNode.filter(s => s.host_asset_id === nodeId).map(ws => ({
      label: ws.name,
      status: ws.status === "up" ? "ok" as const : ws.status === "down" ? "bad" as const : "unknown" as const,
    })),
  ], [protocols, webServicesForNode, nodeId]);

  const panelContext = useMemo(() => buildNodePanelContext({
    asset: asset ?? placeholderAsset,
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
    protocolCount: protocols.length,
    webServiceCount: webServicesForNode.filter(s => s.host_asset_id === nodeId).length,
    connectionBadges,
  }), [
    asset,
    infraCategories,
    isInfra,
    isLinuxAgentNode,
    isPBSAsset,
    isProxmoxAsset,
    isTrueNASAsset,
    mergedDockerHost,
    nodeHasAgent,
    shouldShowDesktop,
    supportsNetworkListing,
    supportsPackageListing,
    supportsScheduleListing,
    supportsServiceListing,
    placeholderAsset,
    protocols,
    webServicesForNode,
    nodeId,
    connectionBadges,
  ]);

  const panels = useMemo(() => {
    if (!asset) {
      return [];
    }
    return buildAvailablePanels(panelContext);
  }, [asset, panelContext]);

  const dockerHostForPanel = useMemo(() => {
    return deriveDockerHostForPanel({
      isDockerHost,
      dockerHostId,
      mergedDockerHostId,
      nodeId,
    });
  }, [dockerHostId, isDockerHost, mergedDockerHostId, nodeId]);

  return {
    asset,
    telemetryOverview,
    freshnessClass,
    groupName,
    metadata,
    proxmoxKind,
    proxmoxNode,
    proxmoxVMID,
    isProxmoxAsset,
    isTrueNASAsset,
    isPBSAsset,
    isDockerHost,
    isDockerContainer,
    isDockerStack,
    dockerHostId,
    dockerContainerId,
    dockerStackName,
    dockerStackAssetId,
    dockerStackHostId,
    canonicalDockerHostNodeID,
    fallbackDockerHostNodeID,
    effectiveKind,
    proxmoxTarget,
    isInfra,
    isLinuxAgentNode,
    supportsServiceListing,
    supportsPackageListing,
    supportsScheduleListing,
    supportsNetworkListing,
    supportsNetworkActions,
    supportsLogQuery,
    logQueryModeLabel,
    nodeNetworkMethodOptions,
    nodeNetworkControlsLabel,
    nodeNetworkControlsHint,
    mergedDockerHost,
    mergedDockerHostId,
    parentInfraHost,
    infraCategories,
    shouldShowDesktop,
    panelContext,
    panels,
    dockerHostForPanel,
  };
}
