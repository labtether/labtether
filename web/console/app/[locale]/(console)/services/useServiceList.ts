"use client";

import { useTranslations } from "next-intl";
import { useMemo } from "react";
import type { WebService } from "../../../hooks/useWebServices";
import { CATEGORY_ORDER, type ServiceLayoutState } from "./servicesLayoutHelpers";
import {
  chipCounts,
  filterServices,
  groupServices,
  isFiltering,
  sortServices,
  type ServiceFilters,
  type ServiceGrouping,
  type ServiceSort,
} from "./servicesListModel";

type Args = {
  services: WebService[];
  view: { filters: ServiceFilters; sort: ServiceSort; grouping: ServiceGrouping };
  assetNameMap: Map<string, string>;
  arranging: boolean;
  arranged: ServiceLayoutState;
};

const byName = (a: [string, string], b: [string, string]) => a[1].localeCompare(b[1]);

/**
 * What the list shows: filtered, sorted and grouped services (visible in display order), the chip counts, the
 * categories and devices the Filters menu offers, and the empty-list sentence. Arranging groups by category and
 * follows the arranged order; so does the default name sort while grouped by category.
 */
export function useServiceList({ services, view, assetNameMap, arranging, arranged }: Args) {
  const t = useTranslations("services.list");
  const { filters, sort } = view;
  return useMemo(() => {
    const started = typeof performance !== "undefined" ? performance.now() : Date.now();
    const grouping: ServiceGrouping = arranging ? "category" : view.grouping;
    const sorted = sortServices(filterServices(services, filters, assetNameMap), sort);
    const useArranged = grouping === "category" && sort.key === "name" && sort.dir === "asc";
    const groups = groupServices(sorted, grouping, assetNameMap, useArranged ? arranged : undefined);

    const categories = [...new Set(services.map((s) => s.category).filter(Boolean))].sort((a, b) => {
      const ai = CATEGORY_ORDER.indexOf(a);
      const bi = CATEGORY_ORDER.indexOf(b);
      return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi) || a.localeCompare(b);
    });
    const hostIDs = [...new Set(services.map((s) => s.host_asset_id).filter(Boolean))];
    const hosts = hostIDs.map((id): [string, string] => [id, assetNameMap.get(id) ?? id]).sort(byName);
    const devices = [...assetNameMap.entries()].sort(byName);

    const query = filters.query.trim();
    const emptyText = query ? t("noMatch", { query })
      : filters.show === "hidden" ? t("noHidden")
        : filters.show === "down" || filters.status === "down" ? t("noneDown")
          : t("noneFiltered");

    return {
      grouping,
      groups,
      visible: groups.flatMap((g) => g.services),
      counts: chipCounts(services),
      categories,
      hosts,
      devices,
      filtering: isFiltering(filters),
      emptyText,
      deriveDurationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - started,
    };
  }, [arranged, arranging, assetNameMap, filters, services, sort, t, view.grouping]);
}
