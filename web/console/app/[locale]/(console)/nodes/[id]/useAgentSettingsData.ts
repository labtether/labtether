"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import {
  buildDraft,
  isServiceDiscoverySetting,
  readJSON,
  type AgentSettingEntry,
  type AgentSettingsHistoryEvent,
  type AgentSettingsHistoryPayload,
  type AgentSettingsPayload,
} from "./agentSettingsModel";

export function useAgentSettingsData({ nodeId }: { nodeId: string }) {
  const [payload, setPayload] = useState<AgentSettingsPayload | null>(null);
  const [history, setHistory] = useState<AgentSettingsHistoryEvent[]>([]);
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [settingsRes, historyRes] = await Promise.all([
        fetch(`/api/agents/${encodeURIComponent(nodeId)}/settings`, { cache: "no-store" }),
        fetch(`/api/agents/${encodeURIComponent(nodeId)}/settings/history`, { cache: "no-store" }),
      ]);

      const settingsData = (await readJSON(settingsRes)) as AgentSettingsPayload & { error?: string };
      if (!settingsRes.ok) {
        throw new Error(settingsData.error || `failed to load agent settings (${settingsRes.status})`);
      }

      const historyData = (await readJSON(historyRes)) as AgentSettingsHistoryPayload & { error?: string };
      if (!historyRes.ok) {
        throw new Error(historyData.error || `failed to load settings history (${historyRes.status})`);
      }

      const settings = Array.isArray(settingsData.settings) ? settingsData.settings : [];
      setPayload({ ...settingsData, settings });
      setHistory(Array.isArray(historyData.events) ? historyData.events : []);
      setDraft(buildDraft(settings));
    } catch (err) {
      setPayload(null);
      setHistory([]);
      setDraft({});
      setError(err instanceof Error ? err.message : "failed to load agent settings");
    } finally {
      setLoading(false);
    }
  }, [nodeId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const editableSettings = useMemo(() => {
    if (!payload) return [] as AgentSettingEntry[];
    return payload.settings.filter((setting) => setting.hub_managed && !setting.local_only);
  }, [payload]);

  const serviceDiscoverySettings = useMemo(() => {
    if (!payload) return [] as AgentSettingEntry[];
    return payload.settings.filter((setting) => isServiceDiscoverySetting(setting.key));
  }, [payload]);

  const generalSettings = useMemo(() => {
    if (!payload) return [] as AgentSettingEntry[];
    return payload.settings.filter((setting) => !isServiceDiscoverySetting(setting.key));
  }, [payload]);

  const changedValues = useMemo(() => {
    const values: Record<string, string> = {};
    for (const setting of editableSettings) {
      const nextValue = draft[setting.key] ?? setting.effective_value;
      if (nextValue !== setting.effective_value) {
        values[setting.key] = nextValue;
      }
    }
    return values;
  }, [draft, editableSettings]);

  const hasPendingChanges = Object.keys(changedValues).length > 0;

  const updateDraftValue = useCallback((key: string, value: string) => {
    setDraft((current) => ({ ...current, [key]: value }));
  }, []);

  return {
    payload,
    history,
    draft,
    loading,
    error,
    setError,
    refresh,
    editableSettings,
    serviceDiscoverySettings,
    generalSettings,
    changedValues,
    hasPendingChanges,
    updateDraftValue,
  };
}
