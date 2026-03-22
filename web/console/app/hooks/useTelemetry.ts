"use client";

import { useEffect, useMemo, useState } from "react";
import { useFastStatus, useStatusSettings } from "../contexts/StatusContext";
import type { AssetTelemetryDetails, TelemetryWindow } from "../console/models";
import { telemetryWindows } from "../console/models";

export function useTelemetry() {
  const status = useFastStatus();
  const { defaultTelemetryWindow } = useStatusSettings();

  const telemetryOverview = useMemo(() => status?.telemetryOverview ?? [], [status?.telemetryOverview]);
  const [telemetryWindow, setTelemetryWindow] = useState<TelemetryWindow>(defaultTelemetryWindow);
  const [selectedTelemetryAsset, setSelectedTelemetryAsset] = useState<string>("");
  const [telemetryDetails, setTelemetryDetails] = useState<AssetTelemetryDetails | null>(null);
  const [telemetryLoading, setTelemetryLoading] = useState(false);
  const [telemetryError, setTelemetryError] = useState<string | null>(null);
  const [hydrated, setHydrated] = useState(false);

  // Hydrate default window from settings
  useEffect(() => {
    if (hydrated) return;
    if (defaultTelemetryWindow !== "1h" || telemetryWindow !== "1h") {
      setTelemetryWindow(defaultTelemetryWindow);
    }
    setHydrated(true);
  }, [defaultTelemetryWindow, hydrated, telemetryWindow]);

  // Auto-select first asset
  useEffect(() => {
    if (telemetryOverview.length === 0) {
      setSelectedTelemetryAsset("");
      return;
    }
    if (selectedTelemetryAsset && telemetryOverview.some((asset) => asset.asset_id === selectedTelemetryAsset)) {
      return;
    }
    setSelectedTelemetryAsset(telemetryOverview[0].asset_id);
  }, [selectedTelemetryAsset, telemetryOverview]);

  // Fetch detail series
  useEffect(() => {
    if (!selectedTelemetryAsset) {
      setTelemetryDetails(null);
      return;
    }

    const controller = new AbortController();
    const load = async () => {
      setTelemetryLoading(true);
      setTelemetryError(null);
      try {
        const response = await fetch(
          `/api/metrics/assets/${encodeURIComponent(selectedTelemetryAsset)}?window=${telemetryWindow}`,
          { cache: "no-store", signal: controller.signal }
        );
        const payload = (await response.json()) as AssetTelemetryDetails & { error?: string };
        if (!response.ok) {
          throw new Error(payload.error || `telemetry fetch failed: ${response.status}`);
        }
        setTelemetryDetails(payload);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setTelemetryError(err instanceof Error ? err.message : "telemetry unavailable");
        setTelemetryDetails(null);
      } finally {
        if (!controller.signal.aborted) {
          setTelemetryLoading(false);
        }
      }
    };

    void load();
    return () => {
      controller.abort();
    };
  }, [selectedTelemetryAsset, telemetryWindow]);

  const selectedOverview = telemetryOverview.find((asset) => asset.asset_id === selectedTelemetryAsset) ?? null;
  const seriesRows = telemetryDetails?.series ?? [];

  return {
    telemetryOverview,
    telemetryWindow,
    setTelemetryWindow,
    telemetryWindows,
    selectedTelemetryAsset,
    setSelectedTelemetryAsset,
    selectedOverview,
    seriesRows,
    telemetryLoading,
    telemetryError
  };
}
