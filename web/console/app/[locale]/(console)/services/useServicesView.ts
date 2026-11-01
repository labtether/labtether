"use client";

import { useCallback, useEffect, useState } from "react";
import {
  DEFAULT_SORT,
  NO_FILTERS,
  SERVICE_GROUPINGS,
  type ServiceFilters,
  type ServiceGrouping,
  type ServiceSort,
  type ServiceSortKey,
} from "./servicesListModel";

const GROUPING_KEY = "labtether.services.grouping";

/** Filters, sort and grouping of the Services list. The grouping is kept in this browser. */
export function useServicesView() {
  const [filters, setFilters] = useState<ServiceFilters>(NO_FILTERS);
  const [sort, setSort] = useState<ServiceSort>(DEFAULT_SORT);
  const [grouping, setGroupingState] = useState<ServiceGrouping>("none");

  useEffect(() => {
    try {
      const stored = window.localStorage.getItem(GROUPING_KEY) as ServiceGrouping | null;
      if (stored && SERVICE_GROUPINGS.includes(stored)) setGroupingState(stored);
    } catch { /* private window: keep the default */ }
  }, []);

  const setGrouping = useCallback((value: ServiceGrouping) => {
    setGroupingState(value);
    try { window.localStorage.setItem(GROUPING_KEY, value); } catch { /* not kept */ }
  }, []);

  const patchFilters = useCallback((patch: Partial<ServiceFilters>) => {
    setFilters((current) => ({ ...current, ...patch }));
  }, []);

  const clearFilters = useCallback(() => setFilters(NO_FILTERS), []);

  /** A column header: the same key flips the direction; a new key starts ascending. */
  const sortBy = useCallback((key: ServiceSortKey) => {
    setSort((current) => current.key === key
      ? { key, dir: current.dir === "asc" ? "desc" : "asc" }
      : { key, dir: "asc" });
  }, []);

  const setSortKey = useCallback((key: ServiceSortKey) => setSort({ key, dir: "asc" }), []);

  return { filters, patchFilters, clearFilters, sort, sortBy, setSortKey, grouping, setGrouping };
}
