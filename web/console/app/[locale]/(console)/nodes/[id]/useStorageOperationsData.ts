"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import type { Asset } from "../../../../console/models";
import { useFastStatus } from "../../../../contexts/StatusContext";
import {
  buildFallbackRows,
  buildInsightRows,
  buildPoolEvents,
  buildRecommendations,
  buildSummary,
  deriveProxmoxStaleInfo,
  deriveStorageAssets,
  filterRows,
  nonEmptyTimelineEvents,
  type ProxmoxStorageDetails,
  type Recommendation,
  type RiskFilter,
  type StorageInsightEvent,
  type StorageInsightsResponse,
  type StorageRow,
} from "./storageOperationsModel";

type UseStorageOperationsDataArgs = {
  hostAsset: Asset;
  proxmoxDetails: ProxmoxStorageDetails | null;
};

export type StorageRiskChip = {
  key: RiskFilter;
  label: string;
  value: number;
};

export function useStorageOperationsData({
  hostAsset,
  proxmoxDetails,
}: UseStorageOperationsDataArgs) {
  const status = useFastStatus();

  const [riskFilter, setRiskFilter] = useState<RiskFilter>("all");
  const [insights, setInsights] = useState<StorageInsightsResponse | null>(null);
  const [insightsLoading, setInsightsLoading] = useState(false);
  const [insightsError, setInsightsError] = useState<string | null>(null);

  const fetchInsights = useCallback(async () => {
    setInsightsLoading(true);
    setInsightsError(null);
    try {
      const res = await fetch(
        `/api/proxmox/assets/${encodeURIComponent(hostAsset.id)}/storage/insights?window=7d`,
        { cache: "no-store" },
      );
      const payload = (await res.json()) as StorageInsightsResponse;
      if (!res.ok) {
        throw new Error(payload.error || `failed to load storage insights (${res.status})`);
      }
      setInsights(payload);
    } catch (err) {
      setInsightsError(err instanceof Error ? err.message : "failed to load storage insights");
      setInsights(null);
    } finally {
      setInsightsLoading(false);
    }
  }, [hostAsset.id]);

  const storageNode = useMemo(() => {
    const fromInsights = insights?.node?.trim() ?? "";
    if (fromInsights !== "") return fromInsights;
    return hostAsset.metadata?.node?.trim() ?? "";
  }, [insights?.node, hostAsset.metadata?.node]);

  const proxmoxCollectorID = useMemo(() => {
    return proxmoxDetails?.collector_id?.trim()
      || hostAsset.metadata?.collector_id?.trim()
      || "";
  }, [proxmoxDetails?.collector_id, hostAsset.metadata?.collector_id]);

  useEffect(() => {
    if (hostAsset.source !== "proxmox") return;
    void fetchInsights();
  }, [hostAsset.source, hostAsset.id, fetchInsights, proxmoxDetails?.fetched_at]);

  const storageAssets = useMemo(
    () => deriveStorageAssets(status?.assets ?? [], hostAsset),
    [status?.assets, hostAsset],
  );

  const proxmoxStaleInfo = useMemo(
    () => deriveProxmoxStaleInfo(proxmoxDetails?.fetched_at),
    [proxmoxDetails?.fetched_at],
  );

  const fallbackRows = useMemo<StorageRow[]>(
    () => buildFallbackRows(storageAssets, proxmoxDetails?.zfs_pools ?? [], proxmoxStaleInfo),
    [storageAssets, proxmoxDetails?.zfs_pools, proxmoxStaleInfo],
  );

  const insightRows = useMemo<StorageRow[]>(
    () => buildInsightRows(insights),
    [insights],
  );

  const rows = insightRows.length > 0 ? insightRows : fallbackRows;

  const rowByKey = useMemo(() => {
    const index = new Map<string, StorageRow>();
    for (const row of rows) {
      index.set(row.key, row);
    }
    return index;
  }, [rows]);

  const timelineEvents = useMemo<StorageInsightEvent[]>(
    () => nonEmptyTimelineEvents(insights),
    [insights],
  );

  const poolEvents = useMemo(
    () => buildPoolEvents(timelineEvents),
    [timelineEvents],
  );

  const summary = useMemo(
    () => buildSummary(rows, insights?.summary),
    [insights?.summary, rows],
  );

  const recommendations = useMemo<Recommendation[]>(
    () => buildRecommendations(rows),
    [rows],
  );

  const filteredRows = useMemo(
    () => filterRows(rows, riskFilter),
    [rows, riskFilter],
  );

  const riskChips = useMemo<StorageRiskChip[]>(() => ([
    { key: "all", label: "All Pools", value: rows.length },
    { key: "degraded", label: "Degraded", value: summary.degraded },
    { key: "hot", label: "Pools >80%", value: summary.hot },
    { key: "predicted", label: "Predicted <30d", value: summary.predicted },
    { key: "scrub", label: "Scrub Overdue", value: summary.scrub },
    { key: "stale", label: "Telemetry Stale", value: summary.stale },
  ]), [rows.length, summary.degraded, summary.hot, summary.predicted, summary.scrub, summary.stale]);

  return {
    riskFilter,
    setRiskFilter,
    insights,
    insightsLoading,
    insightsError,
    fetchInsights,
    storageNode,
    proxmoxCollectorID,
    proxmoxStaleInfo,
    rows,
    rowByKey,
    timelineEvents,
    poolEvents,
    recommendations,
    filteredRows,
    riskChips,
  };
}
