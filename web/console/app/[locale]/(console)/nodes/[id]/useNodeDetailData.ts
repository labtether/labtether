"use client";

import { useNodeActionsData } from "./useNodeActionsData";
import { useNodeLogsData } from "./useNodeLogsData";
import { useNodeProxmoxData } from "./useNodeProxmoxData";
import { useNodeTelemetryData } from "./useNodeTelemetryData";

type UseNodeDetailDataArgs = {
  activeTab: string;
  nodeId: string;
  isProxmoxAsset: boolean;
  isInfra: boolean;
  effectiveKind: string;
  proxmoxNode: string;
  proxmoxTarget: string;
  proxmoxCollectorHint: string;
};

export function useNodeDetailData({
  activeTab,
  nodeId,
  isProxmoxAsset,
  isInfra,
  effectiveKind,
  proxmoxNode,
  proxmoxTarget,
  proxmoxCollectorHint,
}: UseNodeDetailDataArgs) {
  const telemetryData = useNodeTelemetryData({ activeTab, nodeId });
  const logsData = useNodeLogsData({ activeTab, nodeId });
  const actionsData = useNodeActionsData({ activeTab, nodeId });
  const proxmoxData = useNodeProxmoxData({
    activeTab,
    nodeId,
    isProxmoxAsset,
    isInfra,
    effectiveKind,
    proxmoxNode,
    proxmoxTarget,
    proxmoxCollectorHint,
  });

  return {
    telemetryDetails: telemetryData.telemetryDetails,
    telemetryLoading: telemetryData.telemetryLoading,
    telemetryWindow: telemetryData.telemetryWindow,
    setTelemetryWindow: telemetryData.setTelemetryWindow,
    logEvents: logsData.logEvents,
    logsLoading: logsData.logsLoading,
    logsError: logsData.logsError,
    logLevelFilter: logsData.logLevelFilter,
    setLogLevelFilter: logsData.setLogLevelFilter,
    logMode: logsData.logMode,
    setLogMode: logsData.setLogMode,
    logWindow: logsData.logWindow,
    setLogWindow: logsData.setLogWindow,
    journalSince: logsData.journalSince,
    setJournalSince: logsData.setJournalSince,
    journalUntil: logsData.journalUntil,
    setJournalUntil: logsData.setJournalUntil,
    journalUnit: logsData.journalUnit,
    setJournalUnit: logsData.setJournalUnit,
    journalPriority: logsData.journalPriority,
    setJournalPriority: logsData.setJournalPriority,
    journalQuery: logsData.journalQuery,
    setJournalQuery: logsData.setJournalQuery,
    journalLiveTail: logsData.journalLiveTail,
    setJournalLiveTail: logsData.setJournalLiveTail,
    refreshLogs: logsData.refreshLogs,
    actionRuns: actionsData.actionRuns,
    actionsLoading: actionsData.actionsLoading,
    expandedActionId: actionsData.expandedActionId,
    setExpandedActionId: actionsData.setExpandedActionId,
    proxmoxDetails: proxmoxData.proxmoxDetails,
    proxmoxLoading: proxmoxData.proxmoxLoading,
    proxmoxError: proxmoxData.proxmoxError,
    clusterStatus: proxmoxData.clusterStatus,
    networkInterfaces: proxmoxData.networkInterfaces,
    proxmoxCollectorID: proxmoxData.proxmoxCollectorID,
    proxmoxActionRunning: proxmoxData.proxmoxActionRunning,
    proxmoxActionMessage: proxmoxData.proxmoxActionMessage,
    proxmoxActionError: proxmoxData.proxmoxActionError,
    nodeActionRunning: actionsData.nodeActionRunning,
    nodeActionMessage: actionsData.nodeActionMessage,
    nodeActionError: actionsData.nodeActionError,
    nodeNetworkActionRunning: actionsData.nodeNetworkActionRunning,
    nodeNetworkActionMessage: actionsData.nodeNetworkActionMessage,
    nodeNetworkActionError: actionsData.nodeNetworkActionError,
    fetchProxmoxDetails: proxmoxData.fetchProxmoxDetails,
    runProxmoxQuickAction: proxmoxData.runProxmoxQuickAction,
    runNodeQuickAction: actionsData.runNodeQuickAction,
    runNodeNetworkAction: actionsData.runNodeNetworkAction,
  };
}
