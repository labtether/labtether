"use client";

import { useEffect, useState } from "react";
import type { AssetTelemetryDetails, TelemetryWindow } from "../../../../console/models";

type UseNodeTelemetryDataArgs = {
  activeTab: string;
  nodeId: string;
};

export function useNodeTelemetryData({ activeTab, nodeId }: UseNodeTelemetryDataArgs) {
  const [telemetryDetails, setTelemetryDetails] = useState<AssetTelemetryDetails | null>(null);
  const [telemetryLoading, setTelemetryLoading] = useState(false);
  const [telemetryWindow, setTelemetryWindow] = useState<TelemetryWindow>("1h");

  useEffect(() => {
    if (activeTab !== "telemetry" || !nodeId) return;
    const controller = new AbortController();
    setTelemetryDetails(null);
    setTelemetryLoading(true);
    const load = async () => {
      try {
        const res = await fetch(
          `/api/metrics/assets/${encodeURIComponent(nodeId)}?window=${telemetryWindow}`,
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
