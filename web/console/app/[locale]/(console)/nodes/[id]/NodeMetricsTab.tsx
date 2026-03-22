"use client";

import { useEffect, useMemo, useState } from "react";
import { Card } from "../../../../components/ui/Card";
import { SegmentedTabs } from "../../../../components/ui/SegmentedTabs";
import { telemetryWindows } from "../../../../console/models";
import type {
  AssetTelemetryDetails,
  TelemetryWindow,
} from "../../../../console/models";
import { NodeMetricsControls } from "./NodeMetricsControls";
import { MetricPanel, SummaryStat } from "./NodeMetricsPanels";
import {
  analyzeTelemetryDetails,
  formatRange,
} from "./nodeMetricsModel";

type Props = {
  telemetryDetails: AssetTelemetryDetails | null;
  telemetryLoading: boolean;
  telemetryWindow: TelemetryWindow;
  onTelemetryWindowChange: (window: TelemetryWindow) => void;
  requestedFocusMetric?: string | null;
};

export function NodeMetricsTab({
  telemetryDetails,
  telemetryLoading,
  telemetryWindow,
  onTelemetryWindowChange,
  requestedFocusMetric,
}: Props) {
  const analyzed = useMemo(
    () => analyzeTelemetryDetails(telemetryDetails),
    [telemetryDetails]
  );

  const [hiddenMetrics, setHiddenMetrics] = useState<Record<string, boolean>>({});
  const [focusMetric, setFocusMetric] = useState<string | null>(null);

  const metricSignature = useMemo(
    () => analyzed.map((series) => series.metric).join("|"),
    [analyzed]
  );

  useEffect(() => {
    setHiddenMetrics((prev) => {
      const next: Record<string, boolean> = {};
      for (const series of analyzed) {
        next[series.metric] = prev[series.metric] ?? false;
      }
      return next;
    });
  }, [metricSignature, analyzed]);

  const visibleSeries = useMemo(
    () => analyzed.filter((series) => !hiddenMetrics[series.metric]),
    [analyzed, hiddenMetrics]
  );

  useEffect(() => {
    if (!focusMetric) return;
    const stillVisible = visibleSeries.some((series) => series.metric === focusMetric);
    if (!stillVisible) {
      setFocusMetric(visibleSeries[0]?.metric ?? null);
    }
  }, [focusMetric, visibleSeries]);

  useEffect(() => {
    const requested = requestedFocusMetric?.trim();
    if (!requested) return;
    const exists = analyzed.some((series) => series.metric === requested);
    if (!exists) return;
    setHiddenMetrics((prev) => {
      if (!prev[requested]) return prev;
      return { ...prev, [requested]: false };
    });
    setFocusMetric((prev) => (prev === requested ? prev : requested));
  }, [requestedFocusMetric, analyzed]);

  const displayedSeries = useMemo(() => {
    if (!focusMetric) return visibleSeries;
    return visibleSeries.filter((series) => series.metric === focusMetric);
  }, [focusMetric, visibleSeries]);

  const warningMetricCount = useMemo(
    () => analyzed.filter((series) => series.flags.some((flag) => flag.tone !== "info")).length,
    [analyzed]
  );

  const hiddenCount = analyzed.length - visibleSeries.length;
  const infoFlagCount = analyzed.reduce(
    (sum, series) => sum + series.flags.filter((flag) => flag.tone === "info").length,
    0
  );
  const selectedLabel = focusMetric
    ? analyzed.find((series) => series.metric === focusMetric)?.label ?? "Single metric"
    : "All visible metrics";
  const sampleCount = analyzed.reduce((sum, series) => sum + series.rawSampleCount, 0);
  const staleCount = analyzed.filter((series) => series.flags.some((flag) => flag.label === "Stale")).length;

  return (
    <Card className="mb-4">
      <div className="mb-3 flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h2 className="text-sm font-medium text-[var(--text)]">Metrics History</h2>
          {telemetryDetails ? (
            <p className="text-[11px] text-[var(--muted)]">
              {formatRange(telemetryDetails.from, telemetryDetails.to)} · {telemetryDetails.step} cadence
              {warningMetricCount > 0 ? ` · ${warningMetricCount} signals need attention` : " · signal quality healthy"}
            </p>
          ) : null}
        </div>
        <SegmentedTabs
          size="sm"
          value={telemetryWindow}
          options={telemetryWindows.map((window) => ({ id: window, label: window }))}
          onChange={onTelemetryWindowChange}
        />
      </div>

      {telemetryLoading ? (
        <p className="text-sm text-[var(--muted)]">Loading telemetry data...</p>
      ) : analyzed.length > 0 ? (
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-2 lg:grid-cols-5">
            <SummaryStat label="Visible" value={`${visibleSeries.length}/${analyzed.length}`} tone="neutral" />
            <SummaryStat
              label="Warnings"
              value={`${warningMetricCount}`}
              tone={warningMetricCount > 0 ? "warn" : "ok"}
            />
            <SummaryStat label="Info Flags" value={`${infoFlagCount}`} tone="neutral" />
            <SummaryStat label="Stale Feeds" value={`${staleCount}`} tone={staleCount > 0 ? "warn" : "ok"} />
            <SummaryStat label="Samples" value={`${sampleCount}`} tone="neutral" />
          </div>

          <NodeMetricsControls
            analyzed={analyzed}
            visibleSeries={visibleSeries}
            hiddenMetrics={hiddenMetrics}
            focusMetric={focusMetric}
            hiddenCount={hiddenCount}
            selectedLabel={selectedLabel}
            onShowAllMetrics={() => {
              setHiddenMetrics((prev) => {
                const next: Record<string, boolean> = {};
                for (const metric of Object.keys(prev)) {
                  next[metric] = false;
                }
                return next;
              });
            }}
            onToggleMetric={(metric) => {
              setHiddenMetrics((prev) => ({ ...prev, [metric]: !prev[metric] }));
            }}
            onFocusMetricChange={setFocusMetric}
          />

          {visibleSeries.length === 0 ? (
            <div className="rounded-lg border border-dashed border-[var(--line)] p-6 text-center">
              <p className="text-sm font-medium text-[var(--text)]">No metrics selected</p>
              <p className="mt-1 text-xs text-[var(--muted)]">Enable at least one metric to render charts.</p>
            </div>
          ) : null}

          {displayedSeries.length > 0 ? (
            <div className="space-y-3">
              {displayedSeries.map((series) => (
                <MetricPanel key={series.metric} series={series} />
              ))}
            </div>
          ) : null}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center gap-2 py-12">
          <p className="text-sm font-medium text-[var(--text)]">No metrics yet</p>
          <p className="max-w-sm text-center text-xs text-[var(--muted)]">
            No data for this device in the current time range. Try a wider range or check if the device is online.
          </p>
        </div>
      )}
    </Card>
  );
}
