"use client";

import type {
  Asset,
  TelemetryOverviewAsset,
} from "../../../../console/models";
import type { CategoryDef, CategorySlug } from "../../../../console/taxonomy";
import { buildAvailablePanels } from "./devicePanels";
import type { PanelContext } from "./devicePanels";
import { useNodeCapabilities } from "./useNodeCapabilities";

export type GroupLabels = Map<string, string>;

export type UseNodeDetailPageModelArgs = {
  nodeId: string;
  assets: Asset[] | undefined;
  telemetry: TelemetryOverviewAsset[] | undefined;
  groupLabelByID: GroupLabels;
  nodeHasAgent: boolean;
};

export type UseNodeDetailPageModelResult = {
  asset: Asset | null;
  telemetryOverview: TelemetryOverviewAsset | null;
  freshnessClass: "ok" | "pending" | "bad";
  groupName: string;
  metadata: Record<string, string>;
  proxmoxKind: string;
  proxmoxNode: string;
  proxmoxVMID: string;
  isProxmoxAsset: boolean;
  isTrueNASAsset: boolean;
  isPBSAsset: boolean;
  isDockerHost: boolean;
  isDockerContainer: boolean;
  isDockerStack: boolean;
  dockerHostId: string;
  dockerContainerId: string;
  dockerStackName: string;
  dockerStackAssetId: string;
  dockerStackHostId: string;
  canonicalDockerHostNodeID: string;
  fallbackDockerHostNodeID: string;
  effectiveKind: string;
  proxmoxTarget: string;
  isInfra: boolean;
  isLinuxAgentNode: boolean;
  supportsServiceListing: boolean;
  supportsPackageListing: boolean;
  supportsScheduleListing: boolean;
  supportsNetworkListing: boolean;
  supportsNetworkActions: boolean;
  supportsLogQuery: boolean;
  logQueryModeLabel: string;
  nodeNetworkMethodOptions: ReturnType<typeof useNodeCapabilities>["nodeNetworkMethodOptions"];
  nodeNetworkControlsLabel: string;
  nodeNetworkControlsHint: string;
  mergedDockerHost: Asset | null;
  mergedDockerHostId: string;
  parentInfraHost: Asset | null;
  infraCategories: Map<CategorySlug, { def: CategoryDef; count: number }>;
  shouldShowDesktop: boolean;
  panelContext: PanelContext;
  panels: ReturnType<typeof buildAvailablePanels>;
  dockerHostForPanel: string;
};
