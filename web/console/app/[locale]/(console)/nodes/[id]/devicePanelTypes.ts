import type { LucideIcon } from "lucide-react";
import type { Asset } from "../../../../console/models";

export type ConnectionBadge = {
  label: string;
  status: "ok" | "bad" | "unknown";
};

export type PanelDef = {
  id: string;
  label: string;
  icon: LucideIcon;
  summary: (ctx: PanelContext) => string[];
  subTabs?: string[];
  defaultSub?: string;
};

export type PanelContext = {
  asset: Asset;
  nodeHasAgent: boolean;
  mergedDockerHost: Asset | null;
  metadata: Record<string, string>;
  infraCategories: Map<string, { count: number }>;
  isProxmoxAsset: boolean;
  isTrueNASAsset: boolean;
  isPBSAsset: boolean;
  isHomeAssistantAsset: boolean;
  isHomeAssistantHub: boolean;
  isHomeAssistantEntity: boolean;
  isInfra: boolean;
  isLinuxAgentNode: boolean;
  supportsServiceListing: boolean;
  supportsPackageListing: boolean;
  supportsScheduleListing: boolean;
  supportsNetworkListing: boolean;
  desktopEligible: boolean;
  protocolCount: number;
  webServiceCount: number;
  connectionBadges: ConnectionBadge[];
};
