import type { LiveStatusResponse, StatusResponse } from "../../console/models";

function asObject(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function asString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function asNumber(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function ensureArray<T>(value: unknown): T[] {
  return Array.isArray(value) ? value as T[] : [];
}

function normalizeDeadLetterAnalytics(value: unknown): StatusResponse["deadLetterAnalytics"] {
  const raw = asObject(value) ?? {};
  return {
    window: asString(raw.window) || "24h",
    bucket: asString(raw.bucket) || "1h",
    total: asNumber(raw.total),
    rate_per_hour: asNumber(raw.rate_per_hour),
    rate_per_day: asNumber(raw.rate_per_day),
    trend: ensureArray<StatusResponse["deadLetterAnalytics"]["trend"][number]>(raw.trend),
    top_components: ensureArray<StatusResponse["deadLetterAnalytics"]["top_components"][number]>(raw.top_components),
    top_subjects: ensureArray<StatusResponse["deadLetterAnalytics"]["top_subjects"][number]>(raw.top_subjects),
    top_error_classes: ensureArray<StatusResponse["deadLetterAnalytics"]["top_error_classes"][number]>(raw.top_error_classes),
  };
}

export function normalizeLiveStatusResponse(value: unknown): LiveStatusResponse {
  const raw = asObject(value) ?? {};
  const summary = asObject(raw.summary) ?? {};
  return {
    timestamp: asString(raw.timestamp),
    summary: {
      servicesUp: asNumber(summary.servicesUp),
      servicesTotal: asNumber(summary.servicesTotal),
      assetCount: asNumber(summary.assetCount),
      staleAssetCount: asNumber(summary.staleAssetCount),
    },
    endpoints: ensureArray<LiveStatusResponse["endpoints"][number]>(raw.endpoints),
    assets: ensureArray<LiveStatusResponse["assets"][number]>(raw.assets),
    telemetryOverview: ensureArray<LiveStatusResponse["telemetryOverview"][number]>(raw.telemetryOverview),
  };
}

export function normalizeStatusResponse(value: unknown): StatusResponse {
  const raw = asObject(value) ?? {};
  const summary = asObject(raw.summary) ?? {};
  return {
    timestamp: asString(raw.timestamp),
    summary: {
      servicesUp: asNumber(summary.servicesUp),
      servicesTotal: asNumber(summary.servicesTotal),
      connectorCount: asNumber(summary.connectorCount),
      groupCount: asNumber(summary.groupCount),
      assetCount: asNumber(summary.assetCount),
      sessionCount: asNumber(summary.sessionCount),
      auditCount: asNumber(summary.auditCount),
      processedJobs: asNumber(summary.processedJobs),
      actionRunCount: asNumber(summary.actionRunCount),
      updateRunCount: asNumber(summary.updateRunCount),
      deadLetterCount: asNumber(summary.deadLetterCount),
      staleAssetCount: asNumber(summary.staleAssetCount),
      retentionError: asString(summary.retentionError) || undefined,
    },
    endpoints: ensureArray<StatusResponse["endpoints"][number]>(raw.endpoints),
    connectors: ensureArray<StatusResponse["connectors"][number]>(raw.connectors),
    groups: ensureArray<StatusResponse["groups"][number]>(raw.groups),
    assets: ensureArray<StatusResponse["assets"][number]>(raw.assets),
    telemetryOverview: ensureArray<StatusResponse["telemetryOverview"][number]>(raw.telemetryOverview),
    recentLogs: ensureArray<StatusResponse["recentLogs"][number]>(raw.recentLogs),
    logSources: ensureArray<StatusResponse["logSources"][number]>(raw.logSources),
    groupReliability: ensureArray<StatusResponse["groupReliability"][number]>(raw.groupReliability),
    actionRuns: ensureArray<StatusResponse["actionRuns"][number]>(raw.actionRuns),
    updatePlans: ensureArray<StatusResponse["updatePlans"][number]>(raw.updatePlans),
    updateRuns: ensureArray<StatusResponse["updateRuns"][number]>(raw.updateRuns),
    deadLetters: ensureArray<StatusResponse["deadLetters"][number]>(raw.deadLetters),
    deadLetterAnalytics: normalizeDeadLetterAnalytics(raw.deadLetterAnalytics),
    sessions: ensureArray<StatusResponse["sessions"][number]>(raw.sessions),
    recentCommands: ensureArray<StatusResponse["recentCommands"][number]>(raw.recentCommands),
    recentAudit: ensureArray<StatusResponse["recentAudit"][number]>(raw.recentAudit),
    canonical: asObject(raw.canonical) as StatusResponse["canonical"] | undefined,
  };
}

function areUnknownValuesEqual(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) {
    return true;
  }

  if (Array.isArray(left) || Array.isArray(right)) {
    if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) {
      return false;
    }
    for (let index = 0; index < left.length; index += 1) {
      if (!areUnknownValuesEqual(left[index], right[index])) {
        return false;
      }
    }
    return true;
  }

  if (left && right && typeof left === "object" && typeof right === "object") {
    const leftObj = left as Record<string, unknown>;
    const rightObj = right as Record<string, unknown>;
    const leftKeys = Object.keys(leftObj);
    const rightKeys = Object.keys(rightObj);
    if (leftKeys.length !== rightKeys.length) {
      return false;
    }
    for (const key of leftKeys) {
      if (!(key in rightObj)) {
        return false;
      }
      if (!areUnknownValuesEqual(leftObj[key], rightObj[key])) {
        return false;
      }
    }
    return true;
  }

  return false;
}

function areLiveSummaryEqual(
  left: StatusResponse["summary"] | LiveStatusResponse["summary"],
  right: LiveStatusResponse["summary"],
): boolean {
  return (
    left.servicesUp === right.servicesUp
    && left.servicesTotal === right.servicesTotal
    && left.assetCount === right.assetCount
    && left.staleAssetCount === right.staleAssetCount
  );
}

// Compare assets by fields that affect UI decisions, ignoring volatile
// timestamps (last_seen_at changes on every heartbeat but only matters when
// an asset goes stale — the staleAssetCount summary field covers that).
function areLiveAssetsEqual(
  left: LiveStatusResponse["assets"],
  right: LiveStatusResponse["assets"],
): boolean {
  if (left.length !== right.length) return false;
  for (let i = 0; i < left.length; i += 1) {
    const a = left[i];
    const b = right[i];
    if (
      a.id !== b.id
      || a.name !== b.name
      || a.status !== b.status
      || a.type !== b.type
      || a.source !== b.source
      || a.group_id !== b.group_id
      || a.platform !== b.platform
      || a.resource_class !== b.resource_class
      || a.resource_kind !== b.resource_kind
    ) {
      return false;
    }
  }
  return true;
}

// Compare telemetry by identity and rounded metrics so that sub-percent
// fluctuations between poll cycles don't trigger full UI re-renders.
function areLiveTelemetryEqual(
  left: LiveStatusResponse["telemetryOverview"],
  right: LiveStatusResponse["telemetryOverview"],
): boolean {
  if (left.length !== right.length) return false;
  for (let i = 0; i < left.length; i += 1) {
    const a = left[i];
    const b = right[i];
    if (
      a.asset_id !== b.asset_id
      || a.status !== b.status
      || Math.round(a.metrics.cpu_used_percent ?? 0) !== Math.round(b.metrics.cpu_used_percent ?? 0)
      || Math.round(a.metrics.memory_used_percent ?? 0) !== Math.round(b.metrics.memory_used_percent ?? 0)
      || Math.round(a.metrics.disk_used_percent ?? 0) !== Math.round(b.metrics.disk_used_percent ?? 0)
      || Math.round(a.metrics.temperature_celsius ?? 0) !== Math.round(b.metrics.temperature_celsius ?? 0)
    ) {
      return false;
    }
  }
  return true;
}

function areLiveSlicesEqual(current: StatusResponse, live: LiveStatusResponse): boolean {
  return (
    areLiveSummaryEqual(current.summary, live.summary)
    && areUnknownValuesEqual(current.endpoints, live.endpoints)
    && areLiveAssetsEqual(current.assets, live.assets)
    && areLiveTelemetryEqual(current.telemetryOverview, live.telemetryOverview)
  );
}

function areSlowSummaryEqual(
  left: StatusResponse["summary"],
  right: StatusResponse["summary"],
): boolean {
  return (
    left.connectorCount === right.connectorCount
    && left.groupCount === right.groupCount
    && left.sessionCount === right.sessionCount
    && left.auditCount === right.auditCount
    && left.processedJobs === right.processedJobs
    && left.actionRunCount === right.actionRunCount
    && left.updateRunCount === right.updateRunCount
    && left.deadLetterCount === right.deadLetterCount
    && left.retentionError === right.retentionError
  );
}

function areSlowStatusSlicesEqual(left: StatusResponse, right: StatusResponse): boolean {
  return (
    areSlowSummaryEqual(left.summary, right.summary)
    && areUnknownValuesEqual(left.connectors, right.connectors)
    && areUnknownValuesEqual(left.groups, right.groups)
    && areUnknownValuesEqual(left.recentLogs, right.recentLogs)
    && areUnknownValuesEqual(left.logSources, right.logSources)
    && areUnknownValuesEqual(left.groupReliability, right.groupReliability)
    && areUnknownValuesEqual(left.actionRuns, right.actionRuns)
    && areUnknownValuesEqual(left.updatePlans, right.updatePlans)
    && areUnknownValuesEqual(left.updateRuns, right.updateRuns)
    && areUnknownValuesEqual(left.deadLetters, right.deadLetters)
    && areUnknownValuesEqual(left.deadLetterAnalytics, right.deadLetterAnalytics)
    && areUnknownValuesEqual(left.sessions, right.sessions)
    && areUnknownValuesEqual(left.recentCommands, right.recentCommands)
    && areUnknownValuesEqual(left.recentAudit, right.recentAudit)
    && areUnknownValuesEqual(left.canonical, right.canonical)
  );
}

export type StatusMerge = { status: StatusResponse; decision: "synthesized" | "merged" | "unchanged" };

export function mergeLiveStatus(current: StatusResponse | null, full: StatusResponse | null, live: LiveStatusResponse): StatusMerge {
  const base = full ?? current;
  let decision: StatusMerge["decision"] = "merged";
  let nextStatus: StatusResponse;
  if (!base) {
    decision = "synthesized";
    nextStatus = {
      timestamp: live.timestamp,
      summary: {
        ...live.summary,
        connectorCount: 0,
        groupCount: 0,
        sessionCount: 0,
        auditCount: 0,
        processedJobs: 0,
        actionRunCount: 0,
        updateRunCount: 0,
        deadLetterCount: 0,
        retentionError: "",
      },
      endpoints: live.endpoints,
      assets: live.assets,
      telemetryOverview: live.telemetryOverview,
      connectors: [],
      groups: [],
      recentLogs: [],
      logSources: [],
      groupReliability: [],
      actionRuns: [],
      updatePlans: [],
      updateRuns: [],
      deadLetters: [],
      deadLetterAnalytics: {
        window: "24h",
        bucket: "1h",
        total: 0,
        rate_per_hour: 0,
        rate_per_day: 0,
        trend: [],
        top_components: [],
        top_subjects: [],
        top_error_classes: [],
      },
      sessions: [],
      recentCommands: [],
      recentAudit: [],
    };
  } else if (current && areLiveSlicesEqual(current, live) && areSlowStatusSlicesEqual(current, base)) {
    decision = "unchanged";
    nextStatus = current;
  } else {
    nextStatus = {
      ...base,
      timestamp: live.timestamp,
      summary: {
        ...base.summary,
        servicesUp: live.summary.servicesUp,
        servicesTotal: live.summary.servicesTotal,
        assetCount: live.summary.assetCount,
        staleAssetCount: live.summary.staleAssetCount,
      },
      endpoints: live.endpoints,
      assets: live.assets,
      telemetryOverview: live.telemetryOverview,
    };
  }
  return { status: nextStatus, decision };
}

export function mergeFullStatus(current: StatusResponse | null, payload: StatusResponse): StatusMerge {
  let decision: StatusMerge["decision"] = "merged";
  let nextStatus: StatusResponse;
  if (!current) {
    decision = "synthesized";
    nextStatus = payload;
  } else {
    nextStatus = {
      ...payload,
      timestamp: current.timestamp,
      summary: {
        ...payload.summary,
        servicesUp: current.summary.servicesUp,
        servicesTotal: current.summary.servicesTotal,
        assetCount: current.summary.assetCount,
        staleAssetCount: current.summary.staleAssetCount,
      },
      endpoints: current.endpoints,
      assets: current.assets,
      telemetryOverview: current.telemetryOverview,
    };
    if (areSlowStatusSlicesEqual(current, nextStatus) && areLiveSlicesEqual(current, {
      timestamp: nextStatus.timestamp,
      summary: {
        servicesUp: nextStatus.summary.servicesUp,
        servicesTotal: nextStatus.summary.servicesTotal,
        assetCount: nextStatus.summary.assetCount,
        staleAssetCount: nextStatus.summary.staleAssetCount,
      },
      endpoints: nextStatus.endpoints,
      assets: nextStatus.assets,
      telemetryOverview: nextStatus.telemetryOverview,
    })) {
      decision = "unchanged";
      nextStatus = current;
    }
  }
  return { status: nextStatus, decision };
}
