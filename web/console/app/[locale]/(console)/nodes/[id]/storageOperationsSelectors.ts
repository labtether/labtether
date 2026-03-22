import { isHealthyHealth, storageKey } from "./storageOperationsUtils";
import type {
  Recommendation,
  RiskFilter,
  StorageInsightEvent,
  StorageInsightsSummary,
  StorageInsightsResponse,
  StorageRow,
} from "./storageOperationsTypes";

export function nonEmptyTimelineEvents(insights: StorageInsightsResponse | null): StorageInsightEvent[] {
  const raw = insights?.events ?? [];
  return raw.filter((event) => (event.message?.trim() ?? "") !== "");
}

export function buildPoolEvents(timelineEvents: StorageInsightEvent[]): Map<string, StorageInsightEvent[]> {
  const index = new Map<string, StorageInsightEvent[]>();
  for (const event of timelineEvents) {
    const poolKey = storageKey(event.pool ?? "");
    if (poolKey === "") continue;
    if (!index.has(poolKey)) index.set(poolKey, []);
    index.get(poolKey)!.push(event);
  }
  return index;
}

export function buildSummary(rows: StorageRow[], insightsSummary?: StorageInsightsSummary): {
  degraded: number;
  hot: number;
  predicted: number;
  scrub: number;
  stale: number;
} {
  if (insightsSummary) {
    return {
      degraded: insightsSummary.degraded_pools,
      hot: insightsSummary.hot_pools,
      predicted: insightsSummary.predicted_full_lt_30d,
      scrub: insightsSummary.scrub_overdue,
      stale: insightsSummary.stale_telemetry,
    };
  }

  const degraded = rows.filter((row) => !isHealthyHealth(row.health)).length;
  const hot = rows.filter((row) => row.usedPercent != null && row.usedPercent >= 80).length;
  const predicted = rows.filter((row) => row.daysToFull != null && row.daysToFull <= 30).length;
  const scrub = rows.filter((row) => row.scrubOverdue).length;
  const stale = rows.filter((row) => row.stale).length;
  return { degraded, hot, predicted, scrub, stale };
}

export function buildRecommendations(rows: StorageRow[]): Recommendation[] {
  const items: Recommendation[] = [];
  for (const row of rows) {
    if (row.riskState === "healthy") continue;
    const backupTarget = row.vmIDs.length > 0
      ? { kind: "vm" as const, vmid: row.vmIDs[0] }
      : row.ctIDs.length > 0
        ? { kind: "ct" as const, vmid: row.ctIDs[0] }
        : undefined;
    items.push({
      key: `${row.key}-${row.riskState}`,
      rowKey: row.key,
      poolName: row.poolName,
      severity: row.riskState === "critical" ? "critical" : "warning",
      message: row.reason,
      confidence: row.confidence,
      backupTarget,
    });
  }
  return items;
}

export function filterRows(rows: StorageRow[], riskFilter: RiskFilter): StorageRow[] {
  return rows.filter((row) => {
    if (riskFilter === "all") return true;
    if (riskFilter === "degraded") return !isHealthyHealth(row.health);
    if (riskFilter === "hot") return row.usedPercent != null && row.usedPercent >= 80;
    if (riskFilter === "predicted") return row.daysToFull != null && row.daysToFull <= 30;
    if (riskFilter === "scrub") return row.scrubOverdue;
    if (riskFilter === "stale") return row.stale;
    return true;
  });
}
