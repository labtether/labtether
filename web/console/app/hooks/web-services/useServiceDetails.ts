"use client";

import { useCallback, type RefObject } from "react";
import { reportRoutePerfMetric } from "../useRoutePerfTelemetry";
import type { WebService } from "./types";
import { fetchServicePayload } from "./serviceRequest";
import { reconcileWebServiceList } from "./serviceModel";

export function useServiceDetails(host: string | undefined, includeHidden: boolean, servicesRef: RefObject<WebService[]>) {
  const loadServiceDetails = useCallback(async (serviceID: string, hostAssetID?: string) => {
    const startedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
    const { response, payload } = await fetchServicePayload({
      detail: "full",
      hostAssetID: hostAssetID ?? host,
      includeHiddenServices: includeHidden,
      serviceID,
    });
    if (!response.ok) {
      throw new Error(payload?.error ?? `HTTP ${response.status}`);
    }
    const { next: nextServices } = reconcileWebServiceList(payload?.services, servicesRef.current, includeHidden);
    reportRoutePerfMetric({
      route: "services",
      metric: "request.service_detail_fetch",
      durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
      sampleSize: nextServices.length,
      metadata: {
        host_filtered: Boolean(hostAssetID ?? host),
        include_hidden: includeHidden,
      },
    });
    return nextServices[0] ?? null;
  }, [host, includeHidden, servicesRef]);

  return loadServiceDetails;
}
