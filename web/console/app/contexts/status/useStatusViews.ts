"use client";

import { useMemo, useRef } from "react";
import type { StatusResponse } from "../../console/models";

type FastStatusSummary = Pick<StatusResponse["summary"], "servicesUp" | "servicesTotal" | "assetCount" | "staleAssetCount">;
type SlowStatusSummary = Pick<
  StatusResponse["summary"],
  | "connectorCount"
  | "groupCount"
  | "sessionCount"
  | "auditCount"
  | "processedJobs"
  | "actionRunCount"
  | "updateRunCount"
  | "deadLetterCount"
  | "retentionError"
>;

export type FastStatusSlice = Pick<StatusResponse, "timestamp" | "endpoints" | "assets" | "telemetryOverview"> & {
  summary: FastStatusSummary;
};

export type SlowStatusSlice = Pick<
  StatusResponse,
  | "connectors"
  | "groups"
  | "recentLogs"
  | "logSources"
  | "groupReliability"
  | "actionRuns"
  | "updatePlans"
  | "updateRuns"
  | "deadLetters"
  | "deadLetterAnalytics"
  | "sessions"
  | "recentCommands"
  | "recentAudit"
  | "canonical"
> & {
  summary: SlowStatusSummary;
};

function areStringMapsEqual(left: Map<string, string>, right: Map<string, string>): boolean {
  if (left === right) {
    return true;
  }
  if (left.size !== right.size) {
    return false;
  }
  for (const [key, value] of left) {
    if (right.get(key) !== value) {
      return false;
    }
  }
  return true;
}

export function useStatusViews(status: StatusResponse | null) {
  const assetNameMapRef = useRef<Map<string, string>>(new Map());
  const statusTimestamp = status?.timestamp ?? null;
  const statusServicesUp = status?.summary.servicesUp;
  const statusServicesTotal = status?.summary.servicesTotal;
  const statusAssetCount = status?.summary.assetCount;
  const statusStaleAssetCount = status?.summary.staleAssetCount;
  const statusConnectorCount = status?.summary.connectorCount;
  const statusGroupCount = status?.summary.groupCount;
  const statusSessionCount = status?.summary.sessionCount;
  const statusAuditCount = status?.summary.auditCount;
  const statusProcessedJobs = status?.summary.processedJobs;
  const statusActionRunCount = status?.summary.actionRunCount;
  const statusUpdateRunCount = status?.summary.updateRunCount;
  const statusDeadLetterCount = status?.summary.deadLetterCount;
  const statusRetentionError = status?.summary.retentionError;
  const statusEndpoints = status?.endpoints ?? null;
  const statusAssets = status?.assets ?? null;
  const statusTelemetryOverview = status?.telemetryOverview ?? null;
  const statusConnectors = status?.connectors ?? null;
  const statusGroups = status?.groups ?? null;
  const statusRecentLogs = status?.recentLogs ?? null;
  const statusLogSources = status?.logSources ?? null;
  const statusGroupReliability = status?.groupReliability ?? null;
  const statusActionRuns = status?.actionRuns ?? null;
  const statusUpdatePlans = status?.updatePlans ?? null;
  const statusUpdateRuns = status?.updateRuns ?? null;
  const statusDeadLetters = status?.deadLetters ?? null;
  const statusDeadLetterAnalytics = status?.deadLetterAnalytics ?? null;
  const statusSessions = status?.sessions ?? null;
  const statusRecentCommands = status?.recentCommands ?? null;
  const statusRecentAudit = status?.recentAudit ?? null;
  const statusCanonical = status?.canonical ?? null;

  const fastSummary = useMemo<FastStatusSummary | null>(() => {
    if (statusTimestamp == null) {
      return null;
    }
    return {
      servicesUp: statusServicesUp ?? 0,
      servicesTotal: statusServicesTotal ?? 0,
      assetCount: statusAssetCount ?? 0,
      staleAssetCount: statusStaleAssetCount ?? 0,
    };
  }, [
    statusAssetCount,
    statusServicesTotal,
    statusServicesUp,
    statusStaleAssetCount,
    statusTimestamp,
  ]);

  const hasStatus = status !== null;
  const slowSummary = useMemo<SlowStatusSummary | null>(() => {
    if (!hasStatus) {
      return null;
    }
    return {
      connectorCount: statusConnectorCount ?? 0,
      groupCount: statusGroupCount ?? 0,
      sessionCount: statusSessionCount ?? 0,
      auditCount: statusAuditCount ?? 0,
      processedJobs: statusProcessedJobs ?? 0,
      actionRunCount: statusActionRunCount ?? 0,
      updateRunCount: statusUpdateRunCount ?? 0,
      deadLetterCount: statusDeadLetterCount ?? 0,
      retentionError: statusRetentionError,
    };
  }, [
    statusActionRunCount,
    statusAuditCount,
    statusConnectorCount,
    statusDeadLetterCount,
    statusGroupCount,
    statusProcessedJobs,
    statusRetentionError,
    statusSessionCount,
    hasStatus,
    statusUpdateRunCount,
  ]);

  const serviceStatusLabel = useMemo(() => {
    if (!fastSummary) {
      return "Connecting...";
    }
    return `${fastSummary.servicesUp}/${fastSummary.servicesTotal} services online`;
  }, [fastSummary]);

  const fastStatus = useMemo<FastStatusSlice | null>(() => {
    if (!fastSummary || statusTimestamp == null || statusEndpoints == null || statusAssets == null || statusTelemetryOverview == null) {
      return null;
    }
    return {
      timestamp: statusTimestamp,
      summary: fastSummary,
      endpoints: statusEndpoints,
      assets: statusAssets,
      telemetryOverview: statusTelemetryOverview,
    };
  }, [fastSummary, statusAssets, statusEndpoints, statusTelemetryOverview, statusTimestamp]);

  const slowStatus = useMemo<SlowStatusSlice | null>(() => {
    if (
      !slowSummary
      || statusConnectors == null
      || statusGroups == null
      || statusRecentLogs == null
      || statusLogSources == null
      || statusGroupReliability == null
      || statusActionRuns == null
      || statusUpdatePlans == null
      || statusUpdateRuns == null
      || statusDeadLetters == null
      || statusDeadLetterAnalytics == null
      || statusSessions == null
      || statusRecentCommands == null
      || statusRecentAudit == null
    ) {
      return null;
    }
    return {
      summary: slowSummary,
      connectors: statusConnectors,
      groups: statusGroups,
      recentLogs: statusRecentLogs,
      logSources: statusLogSources,
      groupReliability: statusGroupReliability,
      actionRuns: statusActionRuns,
      updatePlans: statusUpdatePlans,
      updateRuns: statusUpdateRuns,
      deadLetters: statusDeadLetters,
      deadLetterAnalytics: statusDeadLetterAnalytics,
      sessions: statusSessions,
      recentCommands: statusRecentCommands,
      recentAudit: statusRecentAudit,
      canonical: statusCanonical ?? undefined,
    };
  }, [
    slowSummary,
    statusActionRuns,
    statusCanonical,
    statusConnectors,
    statusDeadLetterAnalytics,
    statusDeadLetters,
    statusGroupReliability,
    statusGroups,
    statusLogSources,
    statusRecentAudit,
    statusRecentCommands,
    statusRecentLogs,
    statusSessions,
    statusUpdatePlans,
    statusUpdateRuns,
  ]);

  const groupRows = status?.groups;
  const groupLabelByID = useMemo(() => {
    const mapped = new Map<string, string>();
    for (const group of groupRows ?? []) {
      mapped.set(group.id, group.name);
    }
    return mapped;
  }, [groupRows]);
  const assetRows = status?.assets;
  const assetNameMap = useMemo(() => {
    const next = new Map<string, string>();
    for (const asset of assetRows ?? []) {
      next.set(asset.id, asset.name);
    }
    const cached = assetNameMapRef.current;
    if (areStringMapsEqual(cached, next)) {
      return cached;
    }
    assetNameMapRef.current = next;
    return next;
  }, [assetRows]);

  return { fastStatus, slowStatus, serviceStatusLabel, groupLabelByID, assetNameMap };
}
