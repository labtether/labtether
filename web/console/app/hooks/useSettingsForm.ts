"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import type {
  RetentionSettingsPayload,
  RuntimeSettingEntry,
  RuntimeSettingsPayload
} from "../console/models";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";

function normalizeRuntimeSettingEntry(value: unknown): RuntimeSettingEntry | null {
  const raw = ensureRecord(value);
  if (!raw) {
    return null;
  }

  const allowedValues = ensureArray<unknown>(raw.allowed_values)
    .filter((entry): entry is string => typeof entry === "string");
  const source = raw.source === "ui" || raw.source === "docker" || raw.source === "default"
    ? raw.source
    : "default";

  return {
    key: ensureString(raw.key),
    label: ensureString(raw.label),
    description: ensureString(raw.description),
    scope: ensureString(raw.scope),
    type: ensureString(raw.type),
    env_var: ensureString(raw.env_var),
    default_value: ensureString(raw.default_value),
    env_value: ensureString(raw.env_value) || undefined,
    override_value: ensureString(raw.override_value) || undefined,
    effective_value: ensureString(raw.effective_value),
    source,
    allowed_values: allowedValues.length > 0 ? allowedValues : undefined,
    min_int: typeof raw.min_int === "number" && Number.isFinite(raw.min_int) ? raw.min_int : undefined,
    max_int: typeof raw.max_int === "number" && Number.isFinite(raw.max_int) ? raw.max_int : undefined,
  };
}

function normalizeRuntimeSettingList(value: unknown): RuntimeSettingEntry[] {
  return ensureArray<unknown>(value)
    .map(normalizeRuntimeSettingEntry)
    .filter((entry): entry is RuntimeSettingEntry => entry !== null);
}

function normalizeRetentionSettings(value: unknown): RetentionSettingsPayload["settings"] | null {
  const raw = ensureRecord(value);
  if (!raw) {
    return null;
  }

  return {
    logs_window: ensureString(raw.logs_window),
    metrics_window: ensureString(raw.metrics_window),
    audit_window: ensureString(raw.audit_window),
    terminal_window: ensureString(raw.terminal_window),
    action_runs_window: ensureString(raw.action_runs_window),
    update_runs_window: ensureString(raw.update_runs_window),
  };
}

function normalizeRetentionPresetList(value: unknown): RetentionSettingsPayload["presets"] {
  return ensureArray<unknown>(value)
    .map((entry) => {
      const raw = ensureRecord(entry);
      if (!raw) {
        return null;
      }
      const settings = normalizeRetentionSettings(raw.settings);
      if (!settings) {
        return null;
      }
      return {
        id: ensureString(raw.id),
        name: ensureString(raw.name),
        description: ensureString(raw.description),
        settings,
      };
    })
    .filter((entry): entry is RetentionSettingsPayload["presets"][number] => entry !== null);
}

export function useSettingsForm() {
  const [runtimeSettings, setRuntimeSettings] = useState<RuntimeSettingEntry[]>([]);
  const [runtimeDraftValues, setRuntimeDraftValues] = useState<Record<string, string>>({});
  const [runtimeSettingsLoading, setRuntimeSettingsLoading] = useState(false);
  const [runtimeSettingsSaving, setRuntimeSettingsSaving] = useState(false);
  const [runtimeSettingsError, setRuntimeSettingsError] = useState<string | null>(null);
  const [runtimeSettingsMessage, setRuntimeSettingsMessage] = useState<string | null>(null);

  const [retentionSettings, setRetentionSettings] = useState<RetentionSettingsPayload["settings"] | null>(null);
  const [retentionPresets, setRetentionPresets] = useState<RetentionSettingsPayload["presets"]>([]);
  const [retentionDraftValues, setRetentionDraftValues] = useState<Record<string, string>>({});
  const [retentionLoading, setRetentionLoading] = useState(false);
  const [retentionSaving, setRetentionSaving] = useState(false);
  const [retentionMessage, setRetentionMessage] = useState<string | null>(null);

  const loadRuntimeSettings = useCallback(async () => {
    setRuntimeSettingsLoading(true);
    setRuntimeSettingsError(null);
    try {
      const response = await fetch("/api/settings/runtime", { cache: "no-store" });
      const payload = ensureRecord(await response.json().catch(() => null)) as Partial<RuntimeSettingsPayload> | null;
      if (!response.ok) {
        throw new Error(ensureString(payload?.error) || `runtime settings fetch failed: ${response.status}`);
      }
      const settings = normalizeRuntimeSettingList(payload?.settings);
      setRuntimeSettings(settings);
      setRuntimeDraftValues(Object.fromEntries(settings.map((entry) => [entry.key, entry.effective_value])));
    } catch (err) {
      setRuntimeSettingsError(err instanceof Error ? err.message : "runtime settings unavailable");
    } finally {
      setRuntimeSettingsLoading(false);
    }
  }, []);

  const loadRetentionSettings = useCallback(async () => {
    setRetentionLoading(true);
    try {
      const response = await fetch("/api/settings/retention", { cache: "no-store" });
      const payload = ensureRecord(await response.json().catch(() => null)) as Partial<RetentionSettingsPayload> | null;
      if (!response.ok) {
        throw new Error(ensureString(payload?.error) || `retention settings fetch failed: ${response.status}`);
      }
      const settings = normalizeRetentionSettings(payload?.settings);
      setRetentionSettings(settings);
      setRetentionPresets(normalizeRetentionPresetList(payload?.presets));
      if (settings) {
        setRetentionDraftValues({
          logs_window: settings.logs_window,
          metrics_window: settings.metrics_window,
          audit_window: settings.audit_window,
          terminal_window: settings.terminal_window,
          action_runs_window: settings.action_runs_window,
          update_runs_window: settings.update_runs_window
        });
      }
    } catch (err) {
      setRetentionMessage(err instanceof Error ? err.message : "retention settings unavailable");
    } finally {
      setRetentionLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadRuntimeSettings();
    void loadRetentionSettings();
  }, [loadRuntimeSettings, loadRetentionSettings]);

  const saveRuntimeSettings = useCallback(async (keys?: string[]) => {
    const allowedKeys = Array.isArray(keys) && keys.length > 0 ? new Set(keys) : null;
    const values: Record<string, string> = {};
    for (const entry of runtimeSettings) {
      if (allowedKeys && !allowedKeys.has(entry.key)) {
        continue;
      }
      const draft = (runtimeDraftValues[entry.key] ?? entry.effective_value).trim();
      if (draft !== entry.effective_value) {
        values[entry.key] = draft;
      }
    }
    if (Object.keys(values).length === 0) {
      setRuntimeSettingsMessage("No runtime settings changes to save.");
      return;
    }

    setRuntimeSettingsSaving(true);
    setRuntimeSettingsMessage(null);
    setRuntimeSettingsError(null);
    try {
      const response = await fetch("/api/settings/runtime", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ values })
      });
      const payload = ensureRecord(await response.json().catch(() => null)) as Partial<RuntimeSettingsPayload> | null;
      if (!response.ok) {
        throw new Error(ensureString(payload?.error) || `runtime settings update failed: ${response.status}`);
      }
      const settings = normalizeRuntimeSettingList(payload?.settings);
      setRuntimeSettings(settings);
      setRuntimeDraftValues(
        Object.fromEntries(settings.map((entry) => [entry.key, entry.effective_value]))
      );
      setRuntimeSettingsMessage("Runtime settings saved.");
    } catch (err) {
      setRuntimeSettingsError(err instanceof Error ? err.message : "failed to save runtime settings");
    } finally {
      setRuntimeSettingsSaving(false);
    }
  }, [runtimeDraftValues, runtimeSettings]);

  const resetRuntimeSetting = useCallback(async (key: string) => {
    setRuntimeSettingsSaving(true);
    setRuntimeSettingsMessage(null);
    setRuntimeSettingsError(null);
    try {
      const response = await fetch("/api/settings/runtime/reset", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ keys: [key] })
      });
      const payload = ensureRecord(await response.json().catch(() => null)) as Partial<RuntimeSettingsPayload> | null;
      if (!response.ok) {
        throw new Error(ensureString(payload?.error) || `runtime setting reset failed: ${response.status}`);
      }
      const settings = normalizeRuntimeSettingList(payload?.settings);
      setRuntimeSettings(settings);
      setRuntimeDraftValues(
        Object.fromEntries(settings.map((entry) => [entry.key, entry.effective_value]))
      );
      setRuntimeSettingsMessage("Runtime setting reset to Docker/default baseline.");
    } catch (err) {
      setRuntimeSettingsError(err instanceof Error ? err.message : "failed to reset runtime setting");
    } finally {
      setRuntimeSettingsSaving(false);
    }
  }, []);

  const applyRetentionPreset = useCallback(async (presetID: string) => {
    setRetentionSaving(true);
    setRetentionMessage(null);
    try {
      const response = await fetch("/api/settings/retention", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ preset: presetID })
      });
      const payload = ensureRecord(await response.json().catch(() => null)) as Partial<RetentionSettingsPayload> | null;
      if (!response.ok) {
        throw new Error(ensureString(payload?.error) || `retention preset apply failed: ${response.status}`);
      }
      const settings = normalizeRetentionSettings(payload?.settings);
      setRetentionSettings(settings);
      setRetentionPresets(normalizeRetentionPresetList(payload?.presets));
      if (!settings) {
        throw new Error("retention preset response missing settings");
      }
      setRetentionDraftValues({
        logs_window: settings.logs_window,
        metrics_window: settings.metrics_window,
        audit_window: settings.audit_window,
        terminal_window: settings.terminal_window,
        action_runs_window: settings.action_runs_window,
        update_runs_window: settings.update_runs_window
      });
      setRetentionMessage(`Retention preset applied: ${presetID}`);
    } catch (err) {
      setRetentionMessage(err instanceof Error ? err.message : "failed to apply retention preset");
    } finally {
      setRetentionSaving(false);
    }
  }, []);

  const saveRetentionSettings = useCallback(async () => {
    if (!retentionSettings) {
      return;
    }
    setRetentionSaving(true);
    setRetentionMessage(null);
    try {
      const response = await fetch("/api/settings/retention", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(retentionDraftValues)
      });
      const payload = ensureRecord(await response.json().catch(() => null)) as Partial<RetentionSettingsPayload> | null;
      if (!response.ok) {
        throw new Error(ensureString(payload?.error) || `retention settings update failed: ${response.status}`);
      }
      const settings = normalizeRetentionSettings(payload?.settings);
      setRetentionSettings(settings);
      setRetentionPresets(normalizeRetentionPresetList(payload?.presets));
      if (!settings) {
        throw new Error("retention settings response missing settings");
      }
      setRetentionDraftValues({
        logs_window: settings.logs_window,
        metrics_window: settings.metrics_window,
        audit_window: settings.audit_window,
        terminal_window: settings.terminal_window,
        action_runs_window: settings.action_runs_window,
        update_runs_window: settings.update_runs_window
      });
      setRetentionMessage("Retention settings saved.");
    } catch (err) {
      setRetentionMessage(err instanceof Error ? err.message : "failed to save retention settings");
    } finally {
      setRetentionSaving(false);
    }
  }, [retentionDraftValues, retentionSettings]);

  const runtimeSettingsByScope = useMemo(() => {
    const grouped = new Map<string, RuntimeSettingEntry[]>();
    for (const entry of runtimeSettings) {
      const scope = entry.scope || "other";
      const list = grouped.get(scope) ?? [];
      list.push(entry);
      grouped.set(scope, list);
    }
    return Array.from(grouped.entries());
  }, [runtimeSettings]);

  return {
    runtimeSettings,
    runtimeDraftValues,
    setRuntimeDraftValues,
    runtimeSettingsLoading,
    runtimeSettingsSaving,
    runtimeSettingsError,
    runtimeSettingsMessage,
    runtimeSettingsByScope,
    saveRuntimeSettings,
    resetRuntimeSetting,
    retentionSettings,
    retentionPresets,
    retentionDraftValues,
    setRetentionDraftValues,
    retentionLoading,
    retentionSaving,
    retentionMessage,
    applyRetentionPreset,
    saveRetentionSettings,
  };
}
