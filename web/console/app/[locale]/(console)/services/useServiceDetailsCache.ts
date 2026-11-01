"use client";

import { startTransition, useCallback, useEffect, useState } from "react";
import type { WebService } from "../../../hooks/useWebServices";

/**
 * The list loads services in compact form; opening a row loads that service in full once and keeps it while the
 * service is still listed. openID is the row whose details are open.
 */
export function useServiceDetailsCache(
  services: WebService[],
  loadServiceDetails: (serviceID: string, hostAssetID?: string) => Promise<WebService | null>,
) {
  const [byID, setByID] = useState<Map<string, WebService>>(() => new Map());
  const [openID, setOpenID] = useState<string | null>(null);

  useEffect(() => {
    setByID((current) => {
      if (current.size === 0) return current;
      const listed = new Set(services.map((s) => s.id));
      const next = new Map([...current].filter(([id]) => listed.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [services]);

  const load = useCallback(async (service: WebService) => {
    const cached = byID.get(service.id);
    if (cached) return cached;
    const detailed = await loadServiceDetails(service.id, service.host_asset_id);
    if (!detailed) return null;
    startTransition(() => setByID((current) => new Map(current).set(detailed.id, detailed)));
    return detailed;
  }, [byID, loadServiceDetails]);

  const toggleDetails = useCallback((service: WebService) => {
    setOpenID((current) => (current === service.id ? null : service.id));
    // Keep the compact row when the full load fails; the next open tries again.
    void load(service).catch(() => {});
  }, [load]);

  const withDetails = useCallback((service: WebService) => byID.get(service.id) ?? service, [byID]);

  return { openID, setOpenID, toggleDetails, withDetails };
}
