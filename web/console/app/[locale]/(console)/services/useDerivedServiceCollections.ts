"use client";

import { useMemo } from "react";
import type { WebService } from "../../../hooks/useWebServices";
import {
  CATEGORY_ORDER,
  groupServicesByCategory,
  matchesServiceHealthFilter,
  serviceLayoutKey,
  type ServiceHealthFilter,
  type ServiceLayoutState,
  type ServiceSortMode,
} from "./servicesPageHelpers";

interface UseDerivedServiceCollectionsArgs {
  services: WebService[];
  assetNameMap: Map<string, string>;
  categoryFilter: string;
  hostFilter: string;
  statusFilter: string;
  sourceFilter: string;
  healthFilter: ServiceHealthFilter;
  sortMode: ServiceSortMode;
  showHidden: boolean;
  selectionMode: boolean;
  selectedServiceKeys: Set<string>;
  layoutOrderByCategory: ServiceLayoutState;
}

interface DerivedServiceCollections {
  hosts: Map<string, string>;
  activeCategories: string[];
  filtered: WebService[];
  selectedFilteredCount: number;
  bulkEditTargets: WebService[];
  deriveDurationMs: number;
}

export function useDerivedServiceCollections({
  services,
  assetNameMap,
  categoryFilter,
  hostFilter,
  statusFilter,
  sourceFilter,
  healthFilter,
  sortMode,
  showHidden,
  selectionMode,
  selectedServiceKeys,
  layoutOrderByCategory,
}: UseDerivedServiceCollectionsArgs) {
  const derived = useMemo<DerivedServiceCollections>(() => {
    const startedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
    const seen = new Map<string, string>();
    const cats = new Set<string>();
    const nowMS = Date.now();
    const filtered: WebService[] = [];
    const bulkEditTargets: WebService[] = [];
    let selectedFilteredCount = 0;

    for (const svc of services) {
      if (!seen.has(svc.host_asset_id)) {
        seen.set(
          svc.host_asset_id,
          assetNameMap.get(svc.host_asset_id) ?? svc.host_asset_id.slice(0, 8)
        );
      }
      cats.add(svc.category);
      if (categoryFilter !== "All" && svc.category !== categoryFilter) continue;
      if (hostFilter !== "all" && svc.host_asset_id !== hostFilter) continue;
      if (statusFilter !== "all" && svc.status !== statusFilter) continue;
      if (sourceFilter !== "all" && svc.source !== sourceFilter) continue;
      if (!matchesServiceHealthFilter(svc, healthFilter, nowMS)) continue;
      if (!showHidden && svc.metadata?.hidden === "true") continue;

      filtered.push(svc);
      if (selectionMode && selectedServiceKeys.size > 0) {
        if (selectedServiceKeys.has(serviceLayoutKey(svc))) {
          selectedFilteredCount += 1;
          bulkEditTargets.push(svc);
        }
      } else if (!selectionMode) {
        bulkEditTargets.push(svc);
      }
    }

    const activeCategories = Array.from(cats).sort((a, b) => {
      const ai = CATEGORY_ORDER.indexOf(a);
      const bi = CATEGORY_ORDER.indexOf(b);
      return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi);
    });

    return {
      hosts: seen,
      activeCategories,
      filtered,
      selectedFilteredCount,
      bulkEditTargets,
      deriveDurationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
    };
  }, [
    services,
    assetNameMap,
    categoryFilter,
    hostFilter,
    statusFilter,
    sourceFilter,
    healthFilter,
    showHidden,
    selectionMode,
    selectedServiceKeys,
  ]);

  const grouped = useMemo(
    () => groupServicesByCategory(derived.filtered, layoutOrderByCategory, sortMode),
    [derived.filtered, layoutOrderByCategory, sortMode]
  );

  return {
    hosts: derived.hosts,
    activeCategories: derived.activeCategories,
    filtered: derived.filtered,
    grouped,
    selectedFilteredCount: derived.selectedFilteredCount,
    bulkEditTargets: derived.bulkEditTargets,
    deriveDurationMs: derived.deriveDurationMs,
  };
}
