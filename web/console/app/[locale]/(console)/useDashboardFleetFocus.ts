"use client";

import { useEffect, useMemo, useState } from "react";
import type { TelemetryOverviewAsset } from "../../console/models";
import { normalizeStatus, severityScore, type FleetStatus } from "./dashboardPageUtils";

export type ScoredFleetNode = TelemetryOverviewAsset & {
  normalizedStatus: FleetStatus;
  severity: number;
};

export function useDashboardFleetFocus(deviceTelemetry: TelemetryOverviewAsset[]) {
  const [fleetExpanded, setFleetExpanded] = useState(false);

  const scored = useMemo<ScoredFleetNode[]>(() => {
    return deviceTelemetry
      .map((node) => {
        const normalizedStatus = normalizeStatus(node);
        return { ...node, normalizedStatus, severity: severityScore(node, normalizedStatus) };
      })
      .sort((a, b) => b.severity - a.severity);
  }, [deviceTelemetry]);

  const issueNodes = useMemo(() => scored.filter((n) => n.severity > 0), [scored]);
  const healthyNodes = useMemo(() => scored.filter((n) => n.severity === 0), [scored]);
  const hasFleetIssues = issueNodes.length > 0;

  const visibleFleet = useMemo(() => {
    if (!hasFleetIssues) return scored;
    return fleetExpanded ? scored : issueNodes;
  }, [hasFleetIssues, scored, fleetExpanded, issueNodes]);

  useEffect(() => {
    if (!hasFleetIssues) {
      setFleetExpanded(false);
    }
  }, [hasFleetIssues]);

  return {
    fleetExpanded,
    setFleetExpanded,
    healthyNodes,
    hasFleetIssues,
    visibleFleet,
  };
}
