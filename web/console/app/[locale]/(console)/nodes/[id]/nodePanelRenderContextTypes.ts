"use client";

import type { NodePanelRendererContext } from "./nodePanelRenderers";

export type UseNodePanelRenderContextArgs = {
  activePanel: NodePanelRendererContext["activePanel"];
  activeSub: NodePanelRendererContext["activeSub"];
  activeDetail: NodePanelRendererContext["activeDetail"];
  nodeId: NodePanelRendererContext["nodeId"];
  asset: NodePanelRendererContext["asset"];
  metadata: NodePanelRendererContext["metadata"];
  telemetryOverview: NodePanelRendererContext["telemetryOverview"];
  telemetryDetails: NodePanelRendererContext["telemetryDetails"];
  telemetryLoading: NodePanelRendererContext["telemetryLoading"];
  telemetryWindow: NodePanelRendererContext["telemetryWindow"];
  infraCategories: NodePanelRendererContext["infraCategories"];
  isDockerContainer: NodePanelRendererContext["isDockerContainer"];
  isDockerStack: NodePanelRendererContext["isDockerStack"];
  isProxmoxAsset: NodePanelRendererContext["isProxmoxAsset"];
  effectiveKind: NodePanelRendererContext["effectiveKind"];
  dockerHostForPanel: NodePanelRendererContext["dockerHostForPanel"];
  dockerContainerId: NodePanelRendererContext["dockerContainerId"];
  dockerStackHostId: NodePanelRendererContext["dockerStackHostId"];
  dockerStackName: NodePanelRendererContext["dockerStackName"];
  dockerStackAssetId: NodePanelRendererContext["dockerStackAssetId"];
  openPanel: NodePanelRendererContext["openPanel"];
  openSystemDetail: NodePanelRendererContext["openSystemDetail"];
  closeSystemDetail: NodePanelRendererContext["closeSystemDetail"];
  replaceDockerSub: NodePanelRendererContext["replaceDockerSub"];
  telemetryWindowChange: NodePanelRendererContext["nodeMetricsTabProps"]["onTelemetryWindowChange"];
  logsLoading: NodePanelRendererContext["nodeLogsTabCardProps"]["logsLoading"];
  logsError: NodePanelRendererContext["nodeLogsTabCardProps"]["logsError"];
  logEvents: NodePanelRendererContext["nodeLogsTabCardProps"]["logEvents"];
  logLevelFilter: NodePanelRendererContext["nodeLogsTabCardProps"]["logLevelFilter"];
  setLogLevelFilter: NodePanelRendererContext["nodeLogsTabCardProps"]["onLogLevelFilterChange"];
  logMode: NodePanelRendererContext["nodeLogsTabCardProps"]["logMode"];
  setLogMode: NodePanelRendererContext["nodeLogsTabCardProps"]["onLogModeChange"];
  supportsLogQuery: NodePanelRendererContext["nodeLogsTabCardProps"]["supportsLogQuery"];
  logQueryModeLabel: NodePanelRendererContext["nodeLogsTabCardProps"]["logQueryModeLabel"];
  logWindow: NodePanelRendererContext["nodeLogsTabCardProps"]["logWindow"];
  setLogWindow: NodePanelRendererContext["nodeLogsTabCardProps"]["onLogWindowChange"];
  journalSince: NodePanelRendererContext["nodeLogsTabCardProps"]["journalSince"];
  setJournalSince: NodePanelRendererContext["nodeLogsTabCardProps"]["onJournalSinceChange"];
  journalUntil: NodePanelRendererContext["nodeLogsTabCardProps"]["journalUntil"];
  setJournalUntil: NodePanelRendererContext["nodeLogsTabCardProps"]["onJournalUntilChange"];
  journalUnit: NodePanelRendererContext["nodeLogsTabCardProps"]["journalUnit"];
  setJournalUnit: NodePanelRendererContext["nodeLogsTabCardProps"]["onJournalUnitChange"];
  journalPriority: NodePanelRendererContext["nodeLogsTabCardProps"]["journalPriority"];
  setJournalPriority: NodePanelRendererContext["nodeLogsTabCardProps"]["onJournalPriorityChange"];
  journalQuery: NodePanelRendererContext["nodeLogsTabCardProps"]["journalQuery"];
  setJournalQuery: NodePanelRendererContext["nodeLogsTabCardProps"]["onJournalQueryChange"];
  journalLiveTail: NodePanelRendererContext["nodeLogsTabCardProps"]["journalLiveTail"];
  setJournalLiveTail: NodePanelRendererContext["nodeLogsTabCardProps"]["onJournalLiveTailChange"];
  refreshLogs: NodePanelRendererContext["nodeLogsTabCardProps"]["onRefresh"];
  actionsLoading: NodePanelRendererContext["nodeActionsTabCardProps"]["actionsLoading"];
  actionRuns: NodePanelRendererContext["nodeActionsTabCardProps"]["actionRuns"];
  expandedActionId: NodePanelRendererContext["nodeActionsTabCardProps"]["expandedActionId"];
  setExpandedActionId: NodePanelRendererContext["nodeActionsTabCardProps"]["onExpandedActionChange"];
  proxmoxDetails: NodePanelRendererContext["proxmoxDetailsTabProps"]["proxmoxDetails"];
  proxmoxLoading: NodePanelRendererContext["proxmoxDetailsTabProps"]["proxmoxLoading"];
  proxmoxError: NodePanelRendererContext["proxmoxDetailsTabProps"]["proxmoxError"];
  proxmoxNode: NodePanelRendererContext["proxmoxDetailsTabProps"]["proxmoxNode"];
  proxmoxVMID: NodePanelRendererContext["proxmoxDetailsTabProps"]["proxmoxVMID"];
  canRunProxmoxQuickActions: NodePanelRendererContext["proxmoxDetailsTabProps"]["canRunProxmoxQuickActions"];
  proxmoxActionRunning: NodePanelRendererContext["proxmoxDetailsTabProps"]["proxmoxActionRunning"];
  clusterStatus: NodePanelRendererContext["proxmoxDetailsTabProps"]["clusterStatus"];
  networkInterfaces: NodePanelRendererContext["proxmoxDetailsTabProps"]["networkInterfaces"];
  proxmoxCollectorID: NodePanelRendererContext["proxmoxDetailsTabProps"]["proxmoxCollectorID"];
  fetchProxmoxDetails: () => Promise<void> | void;
  runProxmoxQuickAction: (
    actionID: string,
    params?: Record<string, string>,
    target?: string
  ) => Promise<void> | void;
  proxmoxCollectorId: NodePanelRendererContext["settingsTabProps"]["collectorId"];
  proxmoxActionMessage: NodePanelRendererContext["proxmoxActionMessage"];
  proxmoxActionError: NodePanelRendererContext["proxmoxActionError"];
};
