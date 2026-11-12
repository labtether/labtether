"use client";

import type {
  Asset,
  AssetTelemetryDetails,
  TelemetryOverviewAsset,
  MetricWindow,
} from "../../../../console/models";

export type SystemDrilldownView = "cpu" | "memory" | "storage" | "network";

export type SystemPanelProps = {
  nodeId: string;
  asset: Asset;
  telemetry: TelemetryOverviewAsset | null;
  telemetryDetails?: AssetTelemetryDetails | null;
  telemetryLoading?: boolean;
  telemetryWindow?: MetricWindow;
  drilldown?: SystemDrilldownView | null;
  onOpenDrilldown?: (view: SystemDrilldownView) => void;
  onCloseDrilldown?: () => void;
  onOpenPanel?: (panel: string) => void;
};
