"use client";

import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { useStatusControls } from "../../../../contexts/StatusContext";
import { AgentDeviceNameCard } from "./AgentDeviceNameCard";
import { AgentSettingsEditorSection } from "./AgentSettingsEditorSection";
import { AgentSettingsOverviewSection } from "./AgentSettingsOverviewSection";
import {
  formatTime,
  getAgentVersionStatusClass,
  getAgentVersionStatusLabel,
  settingControlSourceStatus,
  settingKeyFilesRootMode,
} from "./agentSettingsModel";
import { useAgentDeviceName } from "./useAgentDeviceName";
import { useAgentSettingsActions } from "./useAgentSettingsActions";
import { useAgentSettingsData } from "./useAgentSettingsData";

export function AgentSettingsTab({ nodeId, assetName }: { nodeId: string; assetName: string }) {
  const { fetchStatus } = useStatusControls();

  const {
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
  } = useAgentSettingsData({ nodeId });

  const {
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
  } = useAgentSettingsActions({
    nodeId,
    draft,
    editableSettings,
    changedValues,
    hasPendingChanges,
    refresh,
    setError,
  });

  const {
    deviceNameDraft,
    setDeviceNameDraft,
    saveDeviceName,
    savingDeviceName,
    normalizedDeviceName,
    hasDeviceNameChange,
    deviceNameError,
    deviceNameMessage,
  } = useAgentDeviceName({ nodeId, assetName, fetchStatus });

  const versionStatusLabel = getAgentVersionStatusLabel(payload?.agent_version_status);
  const versionStatusClass = getAgentVersionStatusClass(payload?.agent_version_status);

  return (
    <Card className="mb-4">
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-medium text-[var(--text)]">Agent Settings</h2>
          <p className="text-xs text-[var(--muted)]">Per-agent runtime and Docker connectivity controls.</p>
        </div>
        <div className="flex items-center gap-2">
          <Badge status={payload?.connected ? "online" : "offline"} size="sm" />
          <Button size="sm" onClick={() => void refresh()} disabled={loading || saving || testingDocker}>
            {loading ? "Refreshing..." : "Refresh"}
          </Button>
        </div>
      </div>

      {error ? <p className="mb-3 text-xs text-[var(--bad)]">{error}</p> : null}
      {message ? <p className="mb-3 text-xs text-[var(--ok)]">{message}</p> : null}

      <AgentDeviceNameCard
        deviceNameDraft={deviceNameDraft}
        onDeviceNameDraftChange={setDeviceNameDraft}
        onSaveDeviceName={() => void saveDeviceName()}
        savingDeviceName={savingDeviceName}
        normalizedDeviceName={normalizedDeviceName}
        hasDeviceNameChange={hasDeviceNameChange}
        deviceNameError={deviceNameError}
        deviceNameMessage={deviceNameMessage}
      />

      {loading ? (
        <p className="text-sm text-[var(--muted)]">Loading agent settings...</p>
      ) : !payload ? (
        <p className="text-sm text-[var(--muted)]">No agent settings available.</p>
      ) : (
        <>
          <AgentSettingsOverviewSection
            payload={payload}
            formatTime={formatTime}
            versionStatusClass={versionStatusClass}
            versionStatusLabel={versionStatusLabel}
            hasPendingChanges={hasPendingChanges}
            saving={saving}
            onSave={() => void saveSettings()}
            onResetOverrides={() => void resetOverrides()}
            testingDocker={testingDocker}
            onTestDocker={() => void testDocker()}
            dockerTestOutput={dockerTestOutput}
            forceAgentUpdate={forceAgentUpdate}
            onForceAgentUpdateChange={setForceAgentUpdate}
            updatingAgent={updatingAgent}
            onUpdateAgent={() => void updateAgent()}
            agentUpdateOutput={agentUpdateOutput}
          />
          <AgentSettingsEditorSection
            serviceDiscoverySettings={serviceDiscoverySettings}
            generalSettings={generalSettings}
            draft={draft}
            settingKeyFilesRootMode={settingKeyFilesRootMode}
            settingControlSourceStatus={settingControlSourceStatus}
            onUpdateDraftValue={updateDraftValue}
            history={history}
            formatTime={formatTime}
          />
        </>
      )}
    </Card>
  );
}
