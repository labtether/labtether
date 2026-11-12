"use client";

import { useEffect, useState } from "react";
import { metricWindowParam } from "../../../../console/models";
import type { AssetTelemetryDetails, MetricWindow } from "../../../../console/models";

type UseNodeTelemetryDataArgs = {
  activeTab: string;
  nodeId: string;
};

export function useNodeTelemetryData({ activeTab, nodeId }: UseNodeTelemetryDataArgs) {
  const [telemetryDetails, setTelemetryDetails] = useState<AssetTelemetryDetails | null>(null);
  const [telemetryLoading, setTelemetryLoading] = useState(false);
  const [telemetryWindow, setTelemetryWindow] = useState<MetricWindow>("24h");

  useEffect(() => {
    // Overview draws the usage charts; the System tab's Metrics view and drill-downs ask as "telemetry".
    if ((activeTab !== "telemetry" && activeTab !== "overview") || !nodeId) return;
    const controller = new AbortController();
    setTelemetryDetails(null);
    setTelemetryLoading(true);
    const load = async () => {
      try {
        const res = await fetch(
          `/api/metrics/assets/${encodeURIComponent(nodeId)}?window=${metricWindowParam(telemetryWindow)}`,
          { cache: "no-store", signal: controller.signal }
        );
        if (res.ok) {
          setTelemetryDetails(await res.json() as AssetTelemetryDetails);
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        // ignore other request failures in polling view
      } finally {
        if (!controller.signal.aborted) setTelemetryLoading(false);
      }
    };
    void load();
    return () => { controller.abort(); };
  }, [activeTab, nodeId, telemetryWindow]);

  return {
    telemetryDetails,
    telemetryLoading,
    telemetryWindow,
    setTelemetryWindow,
  };
}
