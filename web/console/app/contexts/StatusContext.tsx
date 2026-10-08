"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import {
  parseBoolSetting,
  parseIntSetting,
  parseWindowSetting,
  settingValue
} from "../console/formatters";
import { runtimeSettingKeys, telemetryWindows } from "../console/models";
import { useAuth } from "./AuthContext";
import { hasAdminRole } from "../lib/roles";
import type {
  RuntimeSettingEntry,
  RuntimeSettingsPayload,
  StatusResponse,
  TelemetryWindow
} from "../console/models";

import { useStatusData } from "./status/useStatusData";
import { useStatusRefresh } from "./status/useStatusRefresh";
import { useStatusViews, type FastStatusSlice, type SlowStatusSlice } from "./status/useStatusViews";
export type { FastStatusSlice, SlowStatusSlice } from "./status/useStatusViews";

type StatusContextValue = {
  status: StatusResponse | null;
  loading: boolean;
  error: string | null;
  selectedGroupFilter: string;
  setSelectedGroupFilter: (value: string) => void;
  fetchStatus: () => Promise<void>;
  runtimeSettings: RuntimeSettingEntry[];
  pollIntervalSeconds: number;
  defaultTelemetryWindow: TelemetryWindow;
  defaultLogWindow: TelemetryWindow;
  logQueryLimit: number;
  defaultActorID: string;
  defaultActionDryRun: boolean;
  defaultUpdateDryRun: boolean;
  serviceStatusLabel: string;
  groupLabelByID: Map<string, string>;
};

type StatusControlsValue = {
  loading: boolean;
  error: string | null;
  selectedGroupFilter: string;
  setSelectedGroupFilter: (value: string) => void;
  fetchStatus: () => Promise<void>;
};

type StatusSettingsValue = {
  runtimeSettings: RuntimeSettingEntry[];
  pollIntervalSeconds: number;
  defaultTelemetryWindow: TelemetryWindow;
  defaultLogWindow: TelemetryWindow;
  logQueryLimit: number;
  defaultActorID: string;
  defaultActionDryRun: boolean;
  defaultUpdateDryRun: boolean;
};

type StatusFastValue = {
  status: FastStatusSlice | null;
  serviceStatusLabel: string;
};

type StatusSlowValue = {
  status: SlowStatusSlice | null;
  groupLabelByID: Map<string, string>;
};

const StatusContext = createContext<StatusContextValue | null>(null);
const StatusControlsContext = createContext<StatusControlsValue | null>(null);
const StatusSettingsContext = createContext<StatusSettingsValue | null>(null);
const StatusFastContext = createContext<StatusFastValue | null>(null);
const StatusSlowContext = createContext<StatusSlowValue | null>(null);
const StatusAssetNameMapContext = createContext<Map<string, string> | null>(null);

export function StatusProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const canReadRuntimeSettings = hasAdminRole(user?.role);
  const [selectedGroupFilter, setSelectedGroupFilter] = useState<string>("all");
  const [runtimeSettings, setRuntimeSettings] = useState<RuntimeSettingEntry[]>([]);
  const { status, loading, error, fetchStatus, fetchLiveStatus, fetchFullStatus } = useStatusData(selectedGroupFilter);
  const pollIntervalSeconds = useMemo(
    () => parseIntSetting(settingValue(runtimeSettings, runtimeSettingKeys.pollIntervalSeconds, "5"), 5),
    [runtimeSettings]
  );
  const pollIntervalMs = useMemo(() => pollIntervalSeconds * 1000, [pollIntervalSeconds]);
  const defaultTelemetryWindow = useMemo(
    () => parseWindowSetting(settingValue(runtimeSettings, runtimeSettingKeys.defaultTelemetryWindow, "1h"), telemetryWindows, "1h"),
    [runtimeSettings]
  );
  const defaultLogWindow = useMemo(
    () => parseWindowSetting(settingValue(runtimeSettings, runtimeSettingKeys.defaultLogWindow, "1h"), telemetryWindows, "1h"),
    [runtimeSettings]
  );
  const logQueryLimit = useMemo(
    () => parseIntSetting(settingValue(runtimeSettings, runtimeSettingKeys.logQueryLimit, "120"), 120),
    [runtimeSettings]
  );
  const defaultActorID = useMemo(
    () => settingValue(runtimeSettings, runtimeSettingKeys.defaultActorID, "owner"),
    [runtimeSettings]
  );
  const defaultActionDryRun = useMemo(
    () => parseBoolSetting(settingValue(runtimeSettings, runtimeSettingKeys.defaultActionDryRun, "true"), true),
    [runtimeSettings]
  );
  const defaultUpdateDryRun = useMemo(
    () => parseBoolSetting(settingValue(runtimeSettings, runtimeSettingKeys.defaultUpdateDryRun, "true"), true),
    [runtimeSettings]
  );

  const loadRuntimeSettings = useCallback(async () => {
    if (!canReadRuntimeSettings) {
      setRuntimeSettings([]);
      return;
    }
    try {
      const response = await fetch("/api/settings/runtime", { cache: "no-store" });
      if (!response.ok) {
        return;
      }
      const payload = (await response.json().catch(() => null)) as RuntimeSettingsPayload | null;
      setRuntimeSettings(Array.isArray(payload?.settings) ? payload.settings : []);
    } catch {
      // runtime settings unavailable — use defaults
    }
  }, [canReadRuntimeSettings]);

  useEffect(() => {
    void loadRuntimeSettings();
  }, [loadRuntimeSettings]);

  useStatusRefresh(selectedGroupFilter, pollIntervalMs, fetchLiveStatus, fetchFullStatus);
  const { fastStatus, slowStatus, serviceStatusLabel, groupLabelByID, assetNameMap } = useStatusViews(status);

  const controlsValue = useMemo<StatusControlsValue>(
    () => ({
      loading,
      error,
      selectedGroupFilter,
      setSelectedGroupFilter,
      fetchStatus,
    }),
    [loading, error, selectedGroupFilter, fetchStatus],
  );

  const settingsValue = useMemo<StatusSettingsValue>(
    () => ({
      runtimeSettings,
      pollIntervalSeconds,
      defaultTelemetryWindow,
      defaultLogWindow,
      logQueryLimit,
      defaultActorID,
      defaultActionDryRun,
      defaultUpdateDryRun,
    }),
    [
      runtimeSettings,
      pollIntervalSeconds,
      defaultTelemetryWindow,
      defaultLogWindow,
      logQueryLimit,
      defaultActorID,
      defaultActionDryRun,
      defaultUpdateDryRun,
    ],
  );

  const fastValue = useMemo<StatusFastValue>(
    () => ({
      status: fastStatus,
      serviceStatusLabel,
    }),
    [fastStatus, serviceStatusLabel],
  );

  const slowValue = useMemo<StatusSlowValue>(
    () => ({
      status: slowStatus,
      groupLabelByID,
    }),
    [slowStatus, groupLabelByID],
  );

  const value = useMemo<StatusContextValue>(
    () => ({
      status,
      loading,
      error,
      selectedGroupFilter,
      setSelectedGroupFilter,
      fetchStatus,
      runtimeSettings,
      pollIntervalSeconds,
      defaultTelemetryWindow,
      defaultLogWindow,
      logQueryLimit,
      defaultActorID,
      defaultActionDryRun,
      defaultUpdateDryRun,
      serviceStatusLabel,
      groupLabelByID
    }),
    [
      status,
      loading,
      error,
      selectedGroupFilter,
      fetchStatus,
      runtimeSettings,
      pollIntervalSeconds,
      defaultTelemetryWindow,
      defaultLogWindow,
      logQueryLimit,
      defaultActorID,
      defaultActionDryRun,
      defaultUpdateDryRun,
      serviceStatusLabel,
      groupLabelByID
    ]
  );

  return (
    <StatusAssetNameMapContext.Provider value={assetNameMap}>
      <StatusControlsContext.Provider value={controlsValue}>
        <StatusSettingsContext.Provider value={settingsValue}>
          <StatusFastContext.Provider value={fastValue}>
            <StatusSlowContext.Provider value={slowValue}>
              <StatusContext.Provider value={value}>
                {children}
              </StatusContext.Provider>
            </StatusSlowContext.Provider>
          </StatusFastContext.Provider>
        </StatusSettingsContext.Provider>
      </StatusControlsContext.Provider>
    </StatusAssetNameMapContext.Provider>
  );
}

export function useStatus(): StatusContextValue {
  const context = useContext(StatusContext);
  if (!context) {
    throw new Error("useStatus must be used within a StatusProvider");
  }
  return context;
}

export function useStatusControls(): StatusControlsValue {
  const context = useContext(StatusControlsContext);
  if (!context) {
    throw new Error("useStatusControls must be used within a StatusProvider");
  }
  return context;
}

export function useStatusSettings(): StatusSettingsValue {
  const context = useContext(StatusSettingsContext);
  if (!context) {
    throw new Error("useStatusSettings must be used within a StatusProvider");
  }
  return context;
}

export function useFastStatus(): FastStatusSlice | null {
  const context = useContext(StatusFastContext);
  if (!context) {
    throw new Error("useFastStatus must be used within a StatusProvider");
  }
  return context.status;
}

export function useSlowStatus(): SlowStatusSlice | null {
  const context = useContext(StatusSlowContext);
  if (!context) {
    throw new Error("useSlowStatus must be used within a StatusProvider");
  }
  return context.status;
}

export function useServiceStatusLabel(): string {
  const context = useContext(StatusFastContext);
  if (!context) {
    throw new Error("useServiceStatusLabel must be used within a StatusProvider");
  }
  return context.serviceStatusLabel;
}

export function useGroupLabelByID(): Map<string, string> {
  const context = useContext(StatusSlowContext);
  if (!context) {
    throw new Error("useGroupLabelByID must be used within a StatusProvider");
  }
  return context.groupLabelByID;
}

export function useStatusAssetNameMap(): Map<string, string> {
  const context = useContext(StatusAssetNameMapContext);
  if (!context) {
    throw new Error("useStatusAssetNameMap must be used within a StatusProvider");
  }
  return context;
}
