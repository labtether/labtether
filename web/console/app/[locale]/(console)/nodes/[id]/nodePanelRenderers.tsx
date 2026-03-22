"use client";

import type { ComponentProps, ReactNode } from "react";
import type { Asset } from "../../../../console/models";
import type { CategoryDef, CategorySlug } from "../../../../console/taxonomy";
import { AgentSettingsTab } from "./AgentSettingsTab";
import { CronTab } from "./CronTab";
import { DesktopTab } from "./DesktopTab";
import { DisksTab } from "./DisksTab";
import { FilesTab } from "./FilesTab";
import { HomeAssistantTab } from "./HomeAssistantTab";
import { InterfacesTab } from "./InterfacesTab";
import { NodeActionsTabCard } from "./NodeActionsTabCard";
import { NodeLogsTabCard } from "./NodeLogsTabCard";
import { NodeMetricsTab } from "./NodeMetricsTab";
import {
  renderDockerInspectPanel,
  renderDockerLogsPanel,
  renderDockerStackContainersPanel,
  renderDockerStatsPanel,
} from "./nodePanelDockerAssetRenderers";
import { renderDockerPanel } from "./nodePanelDockerRenderer";
import { renderInfraCategoryPanel } from "./nodePanelInfraCategoryRenderer";
import { PackagesTab } from "./PackagesTab";
import { PBSTab } from "./PBSTab";
import { PortainerPanel } from "./PortainerPanel";
import { ProcessesTab } from "./ProcessesTab";
import { ProxmoxDetailsTab } from "./ProxmoxDetailsTab";
import { ServicesTab } from "./ServicesTab";
import { SettingsTab } from "./SettingsTab";
import { SystemPanel, type SystemDrilldownView, type SystemPanelProps } from "./SystemPanel";
import { TerminalTab } from "./TerminalTab";
import { TrueNASTab } from "./TrueNASTab";
import { UsersTab } from "./UsersTab";
import { ConnectPanel } from "./ConnectPanel";

type RegisteredPanelID =
  | "system"
  | "docker"
  | "terminal"
  | "desktop"
  | "monitoring"
  | "processes"
  | "network"
  | "files"
  | "services"
  | "packages"
  | "logs"
  | "actions"
  | "cron"
  | "disks"
  | "users"
  | "agent-settings"
  | "proxmox"
  | "truenas"
  | "pbs"
  | "portainer"
  | "homeassistant"
  | "settings"
  | "stats"
  | "inspect"
  | "containers"
  | "connect";

type StorageActionRunner = (
  actionID: string,
  target: string,
  params?: Record<string, string>,
) => Promise<void> | void;

export type NodePanelRendererContext = {
  activePanel: string | null;
  activeSub: string | null;
  activeDetail: string | null;
  metadata: Record<string, string>;
  nodeId: SystemPanelProps["nodeId"];
  asset: Asset;
  telemetryOverview: SystemPanelProps["telemetry"];
  telemetryDetails: SystemPanelProps["telemetryDetails"];
  telemetryLoading: NonNullable<SystemPanelProps["telemetryLoading"]>;
  telemetryWindow: SystemPanelProps["telemetryWindow"];
  infraCategories: Map<CategorySlug, { def: CategoryDef; count: number }>;
  isDockerContainer: boolean;
  isDockerStack: boolean;
  isProxmoxAsset: boolean;
  effectiveKind: string;
  dockerHostForPanel: string;
  dockerContainerId: string;
  dockerStackHostId: string;
  dockerStackName: string;
  dockerStackAssetId: string;
  openPanel: (panel: string) => void;
  openSystemDetail: (detail: SystemDrilldownView) => void;
  closeSystemDetail: () => void;
  replaceDockerSub: (sub: string) => void;
  nodeMetricsTabProps: Omit<ComponentProps<typeof NodeMetricsTab>, "requestedFocusMetric">;
  nodeLogsTabCardProps: ComponentProps<typeof NodeLogsTabCard>;
  nodeActionsTabCardProps: ComponentProps<typeof NodeActionsTabCard>;
  proxmoxDetailsTabProps: ComponentProps<typeof ProxmoxDetailsTab>;
  settingsTabProps: ComponentProps<typeof SettingsTab>;
  proxmoxActionMessage: string | null;
  proxmoxActionError: string | null;
  onRunStorageProxmoxAction: StorageActionRunner;
};

type PanelRenderer = (context: NodePanelRendererContext) => ReactNode;

const PANEL_RENDERERS: Record<RegisteredPanelID, PanelRenderer> = {
  system: (context) => (
    <SystemPanel
      nodeId={context.nodeId}
      asset={context.asset}
      telemetry={context.telemetryOverview}
      telemetryDetails={context.telemetryDetails}
      telemetryLoading={context.telemetryLoading}
      telemetryWindow={context.telemetryWindow}
      drilldown={normalizeSystemDrilldown(context.activeDetail)}
      onOpenDrilldown={context.openSystemDetail}
      onCloseDrilldown={context.closeSystemDetail}
      onOpenPanel={context.openPanel}
    />
  ),
  docker: renderDockerPanel,
  terminal: (context) => <TerminalTab nodeId={context.nodeId} />,
  desktop: (context) => <DesktopTab nodeId={context.nodeId} />,
  monitoring: (context) => (
    <NodeMetricsTab
      {...context.nodeMetricsTabProps}
      requestedFocusMetric={null}
    />
  ),
  processes: (context) => <ProcessesTab nodeId={context.nodeId} />,
  network: (context) => <InterfacesTab nodeId={context.nodeId} />,
  files: (context) => <FilesTab nodeId={context.nodeId} />,
  services: (context) => <ServicesTab nodeId={context.nodeId} />,
  packages: (context) => (
    <PackagesTab
      nodeId={context.nodeId}
      backend={context.metadata["package_backend"] ?? ""}
    />
  ),
  logs: renderDockerLogsPanel,
  actions: (context) => <NodeActionsTabCard {...context.nodeActionsTabCardProps} />,
  cron: (context) => <CronTab nodeId={context.nodeId} />,
  disks: (context) => <DisksTab nodeId={context.nodeId} />,
  users: (context) => <UsersTab nodeId={context.nodeId} />,
  "agent-settings": (context) => <AgentSettingsTab nodeId={context.nodeId} assetName={context.asset.name} />,
  proxmox: (context) => <ProxmoxDetailsTab {...context.proxmoxDetailsTabProps} />,
  truenas: (context) => <TrueNASTab assetId={context.nodeId} />,
  pbs: (context) => <PBSTab assetId={context.nodeId} />,
  portainer: (context) => <PortainerPanel asset={context.asset} />,
  homeassistant: (context) => <HomeAssistantTab asset={context.asset} />,
  settings: (context) => <SettingsTab {...context.settingsTabProps} />,
  stats: renderDockerStatsPanel,
  inspect: renderDockerInspectPanel,
  containers: renderDockerStackContainersPanel,
  connect: (context) => <ConnectPanel nodeId={context.nodeId} />,
};

function isRegisteredPanelID(panel: string): panel is RegisteredPanelID {
  return panel in PANEL_RENDERERS;
}

function normalizeSystemDrilldown(value: string | null): SystemDrilldownView | null {
  switch (value) {
    case "cpu":
    case "memory":
    case "storage":
    case "network":
      return value;
    default:
      return null;
  }
}

export function renderNodeDetailPanel(context: NodePanelRendererContext): ReactNode {
  const panel = context.activePanel;
  if (!panel) return null;

  if (context.infraCategories.has(panel as CategorySlug)) {
    return renderInfraCategoryPanel(context);
  }

  if (isRegisteredPanelID(panel)) {
    return PANEL_RENDERERS[panel](context);
  }

  return renderInfraCategoryPanel(context);
}
