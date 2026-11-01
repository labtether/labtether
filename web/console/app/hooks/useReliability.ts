"use client";

import { useCallback, useEffect, useState } from "react";
import { useSlowStatus, useStatusControls } from "../contexts/StatusContext";
import type { GroupTimelineResponse, GroupTimelineWindow } from "../console/models";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";

function normalizeGroupTimelineResponse(value: unknown): GroupTimelineResponse | null {
  const raw = ensureRecord(value);
  if (!raw) {
    return null;
  }

  const impact = ensureRecord(raw.impact);
  const reliability = ensureRecord(raw.reliability);
  const group = ensureRecord(raw.group);
  if (!impact || !reliability || !group) {
    return null;
  }

  return {
    ...(raw as GroupTimelineResponse),
    impact: impact as GroupTimelineResponse["impact"],
    reliability: reliability as GroupTimelineResponse["reliability"],
    group: group as GroupTimelineResponse["group"],
    events: ensureArray<GroupTimelineResponse["events"][number]>(raw.events),
  };
}

const EMPTY_ANALYTICS = {
  window: "24h",
  bucket: "1h",
  total: 0,
  rate_per_hour: 0,
  rate_per_day: 0,
  trend: [],
  top_components: [],
  top_subjects: [],
  top_error_classes: [],
};

/**
 * The Health page's status data (group scores, failed jobs, data cleanup) and one group's event timeline. Downtime
 * windows load separately for every group (reliability/useDowntimeWindows).
 */
export function useReliability() {
  const status = useSlowStatus();
  const { selectedGroupFilter } = useStatusControls();

  const groupRows = status?.groups ?? [];
  const groupReliabilityRows = status?.groupReliability ?? [];
  const deadLetters = status?.deadLetters ?? [];
  const deadLetterAnalytics = status?.deadLetterAnalytics ?? EMPTY_ANALYTICS;
  const retentionError = status?.summary.retentionError ?? "";

  const [selectedTimelineGroup, setSelectedTimelineGroup] = useState<string>("");
  const [groupTimelineWindow, setGroupTimelineWindow] = useState<GroupTimelineWindow>("24h");
  const [groupTimeline, setGroupTimeline] = useState<GroupTimelineResponse | null>(null);
  const [groupTimelineLoading, setGroupTimelineLoading] = useState(false);
  const [groupTimelineError, setGroupTimelineError] = useState<string | null>(null);

  // Auto-select timeline group
  useEffect(() => {
    const availableGroups = status?.groups ?? [];
    if (availableGroups.length === 0) {
      setSelectedTimelineGroup("");
      return;
    }
    if (selectedGroupFilter !== "all" && availableGroups.some((group) => group.id === selectedGroupFilter)) {
      setSelectedTimelineGroup(selectedGroupFilter);
      return;
    }
    if (selectedTimelineGroup && availableGroups.some((group) => group.id === selectedTimelineGroup)) {
      return;
    }
    setSelectedTimelineGroup(availableGroups[0].id);
  }, [selectedGroupFilter, selectedTimelineGroup, status?.groups]);

  const loadGroupTimeline = useCallback(async () => {
    if (!selectedTimelineGroup) {
      setGroupTimeline(null);
      return;
    }

    setGroupTimelineLoading(true);
    setGroupTimelineError(null);
    try {
      const response = await fetch(
        `/api/groups/${encodeURIComponent(selectedTimelineGroup)}/timeline?window=${groupTimelineWindow}&limit=60`,
        { cache: "no-store" }
      );
      const payload = ensureRecord(await response.json().catch(() => null));
      if (!response.ok) {
        throw new Error(ensureString(payload?.error) || `group timeline fetch failed: ${response.status}`);
      }
      setGroupTimeline(normalizeGroupTimelineResponse(payload));
    } catch (err) {
      setGroupTimeline(null);
      setGroupTimelineError(err instanceof Error ? err.message : "group timeline unavailable");
    } finally {
      setGroupTimelineLoading(false);
    }
  }, [selectedTimelineGroup, groupTimelineWindow]);

  useEffect(() => {
    void loadGroupTimeline();
  }, [loadGroupTimeline]);

  return {
    groupRows,
    groupReliabilityRows,
    deadLetters,
    deadLetterAnalytics,
    retentionError,
    selectedTimelineGroup,
    setSelectedTimelineGroup,
    groupTimelineWindow,
    setGroupTimelineWindow,
    groupTimeline,
    groupTimelineLoading,
    groupTimelineError,
    reloadGroupTimeline: loadGroupTimeline,
  };
}
