"use client";

import { useEffect, useMemo } from "react";
import { Cpu, HardDrive, MemoryStick, Network } from "lucide-react";
import { buildNodeMetadataSections } from "../../../../console/models";
import { useNetworkInterfaces } from "../../../../hooks/useNetworkInterfaces";
import { DataValue } from "../../../../components/ui/DataValue";
import { SummaryMetricCard, SystemPanelDrilldown } from "./SystemPanelDrilldown";
import { analyzeTelemetryDetails } from "./nodeMetricsModel";
import {
  CURATED_SECTIONS,
  buildDrilldownContent,
  formatBytes,
  formatPercent,
  formatRate,
  gatherMetrics,
  readMeta,
  relevantHistorySeries,
} from "./systemPanelModel";
import type { SystemPanelProps } from "./systemPanelTypes";

export type { SystemDrilldownView, SystemPanelProps } from "./systemPanelTypes";

export function SystemPanel({
  nodeId,
  asset,
  telemetry,
  telemetryDetails = null,
  telemetryLoading = false,
  telemetryWindow,
  drilldown = null,
  onOpenDrilldown,
  onCloseDrilldown,
  onOpenPanel,
}: SystemPanelProps) {
  const sections = buildNodeMetadataSections(asset.metadata);
  const curated = sections.filter((section) => CURATED_SECTIONS.includes(section.title));
  const metrics = gatherMetrics(asset, telemetry);
  const analyzedSeries = useMemo(() => analyzeTelemetryDetails(telemetryDetails), [telemetryDetails]);
  const historySeries = useMemo(
    () => (drilldown ? relevantHistorySeries(analyzedSeries, drilldown) : []),
    [analyzedSeries, drilldown]
  );
  const {
    interfaces,
    loading: interfacesLoading,
    error: interfacesError,
    refresh: refreshInterfaces,
  } = useNetworkInterfaces(nodeId);

  useEffect(() => {
    if (drilldown !== "network") {
      return;
    }
    void refreshInterfaces();
  }, [drilldown, refreshInterfaces]);

  if (drilldown) {
    return (
      <SystemPanelDrilldown
        content={buildDrilldownContent(asset, metrics, drilldown)}
        drilldown={drilldown}
        historySeries={historySeries}
        telemetryLoading={telemetryLoading}
        telemetryWindow={telemetryWindow}
        onOpenDrilldown={onOpenDrilldown}
        onCloseDrilldown={onCloseDrilldown}
        onOpenPanel={onOpenPanel}
        interfaces={interfaces}
        interfacesLoading={interfacesLoading}
        interfacesError={interfacesError}
      />
    );
  }

  if (curated.length === 0) {
    return (
      <div className="rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] p-6 text-center text-sm text-[var(--muted)]">
        No system information available for this device.
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
        <SummaryMetricCard
          icon={<Cpu size={15} />}
          title="CPU"
          value={formatPercent(metrics.cpu_used_percent)}
          hint={readMeta(asset, "cpu_model") || "Open CPU model and load diagnostics"}
          onClick={() => onOpenDrilldown?.("cpu")}
        />
        <SummaryMetricCard
          icon={<MemoryStick size={15} />}
          title="Memory"
          value={formatPercent(metrics.memory_used_percent)}
          hint={formatBytes(readMeta(asset, "memory_total_bytes") || readMeta(asset, "physmem")) || "Open memory diagnostics"}
          onClick={() => onOpenDrilldown?.("memory")}
        />
        <SummaryMetricCard
          icon={<HardDrive size={15} />}
          title="Storage"
          value={formatPercent(metrics.disk_used_percent)}
          hint={formatBytes(readMeta(asset, "disk_root_total_bytes")) || "Open storage diagnostics"}
          onClick={() => onOpenDrilldown?.("storage")}
        />
        <SummaryMetricCard
          icon={<Network size={15} />}
          title="Network"
          value={`${formatRate(metrics.network_rx_bytes_per_sec)} / ${formatRate(metrics.network_tx_bytes_per_sec)}`}
          hint={`${readMeta(asset, "network_interface_count") || "n/a"} interfaces`}
          onClick={() => onOpenDrilldown?.("network")}
        />
      </div>

      {curated.map((section) => (
        <div
          key={section.title}
          className="rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] px-4 py-3"
        >
          <h3 className="mb-2 text-sm font-medium text-[var(--text)]">
            {section.title}
          </h3>
          <div className="grid grid-cols-1 gap-x-6 gap-y-1.5 sm:grid-cols-2">
            {section.rows.map((row) => (
              <div key={row.key} className="flex min-w-0 items-baseline gap-2">
                <span className="w-[140px] shrink-0 text-[11px] text-[var(--muted)]">
                  {row.label}
                </span>
                <DataValue
                  value={row.value}
                  className="truncate text-xs text-[var(--text)]"
                />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
