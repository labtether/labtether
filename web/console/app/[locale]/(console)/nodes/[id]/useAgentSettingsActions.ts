"use client";

import { useCallback, useState } from "react";

import {
  readError,
  readJSON,
  settingKeyDockerEnabled,
  settingKeyDockerEndpoint,
  type AgentSettingEntry,
  type AgentUpdateResponse,
  type DockerTestResponse,
} from "./agentSettingsModel";

type UseAgentSettingsActionsArgs = {
  nodeId: string;
  draft: Record<string, string>;
  editableSettings: AgentSettingEntry[];
  changedValues: Record<string, string>;
  hasPendingChanges: boolean;
  refresh: () => Promise<void>;
  setError: (value: string | null) => void;
};

export function useAgentSettingsActions({
  nodeId,
  draft,
  editableSettings,
  changedValues,
  hasPendingChanges,
  refresh,
  setError,
}: UseAgentSettingsActionsArgs) {
  const [saving, setSaving] = useState(false);
  const [testingDocker, setTestingDocker] = useState(false);
  const [updatingAgent, setUpdatingAgent] = useState(false);
  const [forceAgentUpdate, setForceAgentUpdate] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [dockerTestOutput, setDockerTestOutput] = useState<string | null>(null);
  const [agentUpdateOutput, setAgentUpdateOutput] = useState<string | null>(null);

  const saveSettings = useCallback(async () => {
    if (!hasPendingChanges) {
      setMessage("No changes to save.");
      return;
    }

    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const response = await fetch(`/api/agents/${encodeURIComponent(nodeId)}/settings`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ values: changedValues }),
      });
      const responsePayload = await readJSON(response);
      if (!response.ok) {
        throw new Error(readError(responsePayload) || "failed to save settings");
      }
      setMessage("Settings saved. Waiting for agent apply acknowledgment.");
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to save settings");
    } finally {
      setSaving(false);
    }
  }, [changedValues, hasPendingChanges, nodeId, refresh, setError]);

  const resetOverrides = useCallback(async () => {
    if (editableSettings.length === 0) return;
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const response = await fetch(`/api/agents/${encodeURIComponent(nodeId)}/settings/reset`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ keys: editableSettings.map((setting) => setting.key) }),
      });
      const responsePayload = await readJSON(response);
      if (!response.ok) {
        throw new Error(readError(responsePayload) || "failed to reset overrides");
      }
      setMessage("Per-agent overrides reset.");
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to reset overrides");
    } finally {
      setSaving(false);
    }
  }, [editableSettings, nodeId, refresh, setError]);

  const testDocker = useCallback(async () => {
    setTestingDocker(true);
    setError(null);
    setDockerTestOutput(null);
    try {
      const enabled = draft[settingKeyDockerEnabled] ?? "";
      const endpoint = draft[settingKeyDockerEndpoint] ?? "";
      const response = await fetch(`/api/agents/${encodeURIComponent(nodeId)}/settings/test-docker`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled, endpoint }),
      });
      const payload = (await readJSON(response)) as DockerTestResponse;
      if (!response.ok) {
        const details = payload.output || payload.error || payload.message || "docker test failed";
        throw new Error(details);
      }
      const summary = [
        payload.message || (payload.ok ? "Docker endpoint check succeeded." : "Docker endpoint check failed."),
        payload.endpoint ? `Endpoint: ${payload.endpoint}` : "",
        payload.status ? `Status: ${payload.status}` : "",
        payload.output ? payload.output : "",
      ]
        .filter(Boolean)
        .join("\n");
      setDockerTestOutput(summary);
    } catch (err) {
      setDockerTestOutput(err instanceof Error ? err.message : "docker test failed");
    } finally {
      setTestingDocker(false);
    }
  }, [draft, nodeId, setError]);

  const updateAgent = useCallback(async () => {
    setUpdatingAgent(true);
    setError(null);
    setAgentUpdateOutput(null);
    setMessage(null);
    try {
      const response = await fetch(`/api/agents/${encodeURIComponent(nodeId)}/settings/update-agent`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ force: forceAgentUpdate }),
      });
      const payload = (await readJSON(response)) as AgentUpdateResponse;
      const summary = payload.summary
        || payload.error
        || (payload.ok ? "Agent self-update completed." : "Agent self-update failed.");
      const detail = [summary, payload.output || ""].filter(Boolean).join("\n\n");
      setAgentUpdateOutput(detail || null);

      if (!response.ok) {
        throw new Error(summary);
      }

      const statusSuffix = payload.agent_disconnected_expected
        ? " The agent may disconnect briefly while restarting."
        : "";
      setMessage(`${summary}${statusSuffix}`);
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to update agent");
    } finally {
      setUpdatingAgent(false);
    }
  }, [forceAgentUpdate, nodeId, refresh, setError]);

  return {
    saving,
    testingDocker,
    updatingAgent,
    forceAgentUpdate,
    setForceAgentUpdate,
    message,
    dockerTestOutput,
    agentUpdateOutput,
    saveSettings,
    resetOverrides,
    testDocker,
    updateAgent,
  };
}
