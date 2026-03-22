"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { useGroupLabelByID, useSlowStatus, useStatusControls } from "../contexts/StatusContext";
import { groupTimelineWindows } from "../console/models";
import type { MaintenanceWindow, GroupTimelineResponse, GroupTimelineWindow } from "../console/models";
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

export function useReliability() {
  const status = useSlowStatus();
  const { fetchStatus, selectedGroupFilter } = useStatusControls();
  const groupLabelByID = useGroupLabelByID();

  const groupRows = status?.groups ?? [];
  const groupReliabilityRows = status?.groupReliability ?? [];
  const deadLetters = status?.deadLetters ?? [];
  const deadLetterAnalytics = status?.deadLetterAnalytics ?? {
    window: "24h",
    bucket: "1h",
    total: 0,
    rate_per_hour: 0,
    rate_per_day: 0,
    trend: [],
    top_components: [],
    top_subjects: [],
    top_error_classes: []
  };

  const [selectedTimelineGroup, setSelectedTimelineGroup] = useState<string>("");
  const [groupTimelineWindow, setGroupTimelineWindow] = useState<GroupTimelineWindow>("24h");
  const [groupTimeline, setGroupTimeline] = useState<GroupTimelineResponse | null>(null);
  const [groupTimelineLoading, setGroupTimelineLoading] = useState(false);
  const [groupTimelineError, setGroupTimelineError] = useState<string | null>(null);
  const [maintenanceWindows, setMaintenanceWindows] = useState<MaintenanceWindow[]>([]);
  const [maintenanceLoading, setMaintenanceLoading] = useState(false);
  const [maintenanceMessage, setMaintenanceMessage] = useState<string | null>(null);
  const [maintenanceName, setMaintenanceName] = useState<string>("Patch Window");
  const [maintenanceStart, setMaintenanceStart] = useState<string>("");
  const [maintenanceEnd, setMaintenanceEnd] = useState<string>("");
  const [maintenanceSuppressAlerts, setMaintenanceSuppressAlerts] = useState<boolean>(true);
  const [maintenanceBlockActions, setMaintenanceBlockActions] = useState<boolean>(false);
  const [maintenanceBlockUpdates, setMaintenanceBlockUpdates] = useState<boolean>(false);
  const [maintenanceSaving, setMaintenanceSaving] = useState(false);

  // Default maintenance start/end
  useEffect(() => {
    if (maintenanceStart || maintenanceEnd) return;
    const now = new Date();
    const start = new Date(now.getTime() + 15 * 60_000);
    const end = new Date(start.getTime() + 60 * 60_000);
    const toLocalInput = (value: Date) => {
      const offset = value.getTimezoneOffset() * 60_000;
      return new Date(value.getTime() - offset).toISOString().slice(0, 16);
    };
    setMaintenanceStart(toLocalInput(start));
    setMaintenanceEnd(toLocalInput(end));
  }, [maintenanceStart, maintenanceEnd]);

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

  const loadMaintenanceWindows = useCallback(async () => {
    if (!selectedTimelineGroup) {
      setMaintenanceWindows([]);
      return;
    }
    setMaintenanceLoading(true);
    setMaintenanceMessage(null);
    try {
      const response = await fetch(`/api/groups/${encodeURIComponent(selectedTimelineGroup)}/maintenance-windows?limit=20`, {
        cache: "no-store"
      });
      const payload = ensureRecord(await response.json().catch(() => null));
      if (!response.ok) {
        throw new Error(ensureString(payload?.error) || `maintenance windows fetch failed: ${response.status}`);
      }
      setMaintenanceWindows(ensureArray<MaintenanceWindow>(payload?.windows));
    } catch (err) {
      setMaintenanceWindows([]);
      setMaintenanceMessage(err instanceof Error ? err.message : "maintenance windows unavailable");
    } finally {
      setMaintenanceLoading(false);
    }
  }, [selectedTimelineGroup]);

  useEffect(() => {
    void loadGroupTimeline();
  }, [loadGroupTimeline]);

  useEffect(() => {
    void loadMaintenanceWindows();
  }, [loadMaintenanceWindows]);

  const createMaintenanceWindow = async (event: FormEvent) => {
    event.preventDefault();
    if (!selectedTimelineGroup) {
      setMaintenanceMessage("Choose a group first.");
      return;
    }
    if (!maintenanceName.trim() || !maintenanceStart || !maintenanceEnd) {
      setMaintenanceMessage("Name, start, and end are required.");
      return;
    }

    setMaintenanceSaving(true);
    setMaintenanceMessage(null);
    try {
      const response = await fetch(`/api/groups/${encodeURIComponent(selectedTimelineGroup)}/maintenance-windows`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: maintenanceName.trim(),
          start_at: new Date(maintenanceStart).toISOString(),
          end_at: new Date(maintenanceEnd).toISOString(),
          suppress_alerts: maintenanceSuppressAlerts,
          block_actions: maintenanceBlockActions,
          block_updates: maintenanceBlockUpdates
        })
      });
      const payload = ensureRecord(await response.json().catch(() => null));
      if (!response.ok) {
        throw new Error(ensureString(payload?.error) || `maintenance window create failed: ${response.status}`);
      }
      setMaintenanceMessage("Maintenance window created.");
      await loadMaintenanceWindows();
      await loadGroupTimeline();
      await fetchStatus();
    } catch (err) {
      setMaintenanceMessage(err instanceof Error ? err.message : "failed to create maintenance window");
    } finally {
      setMaintenanceSaving(false);
    }
  };

  const deleteMaintenanceWindow = async (windowId: string) => {
    if (!selectedTimelineGroup) return;
    setMaintenanceMessage(null);
    try {
      const response = await fetch(
        `/api/groups/${encodeURIComponent(selectedTimelineGroup)}/maintenance-windows/${encodeURIComponent(windowId)}`,
        { method: "DELETE" }
      );
      const payload = ensureRecord(await response.json().catch(() => null));
      if (!response.ok) {
        throw new Error(ensureString(payload?.error) || `maintenance window delete failed: ${response.status}`);
      }
      setMaintenanceMessage("Maintenance window deleted.");
      await loadMaintenanceWindows();
      await loadGroupTimeline();
      await fetchStatus();
    } catch (err) {
      setMaintenanceMessage(err instanceof Error ? err.message : "failed to delete maintenance window");
    }
  };

  return {
    groupRows,
    groupReliabilityRows,
    groupLabelByID,
    deadLetters,
    deadLetterAnalytics,
    selectedTimelineGroup,
    setSelectedTimelineGroup,
    groupTimelineWindow,
    setGroupTimelineWindow,
    groupTimelineWindows,
    groupTimeline,
    groupTimelineLoading,
    groupTimelineError,
    maintenanceWindows,
    maintenanceLoading,
    maintenanceMessage,
    maintenanceName,
    setMaintenanceName,
    maintenanceStart,
    setMaintenanceStart,
    maintenanceEnd,
    setMaintenanceEnd,
    maintenanceSuppressAlerts,
    setMaintenanceSuppressAlerts,
    maintenanceBlockActions,
    setMaintenanceBlockActions,
    maintenanceBlockUpdates,
    setMaintenanceBlockUpdates,
    maintenanceSaving,
    createMaintenanceWindow,
    deleteMaintenanceWindow
  };
}
