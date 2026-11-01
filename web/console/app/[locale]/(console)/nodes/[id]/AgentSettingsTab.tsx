"use client";

import { RefreshCw } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { useStatusControls } from "../../../../contexts/StatusContext";
import { AgentDeviceNameCard } from "./AgentDeviceNameCard";
import { AgentSettingsEditorSection } from "./AgentSettingsEditorSection";
import { AgentSettingsOverviewSection } from "./AgentSettingsOverviewSection";
import { formatTime, settingKeyFilesRootMode } from "./agentSettingsModel";
import { useAgentDeviceName } from "./useAgentDeviceName";
import { useAgentSettingsActions } from "./useAgentSettingsActions";
import { useAgentSettingsData } from "./useAgentSettingsData";

/** Settings › Agent: the agent's facts and updates, the device name it keeps, its settings and their apply history. */
export function AgentSettingsTab({ nodeId, assetName }: { nodeId: string; assetName: string }) {
  const t = useTranslations("devices.settingsTab.agent");
  const tSet = useTranslations("devices.settingsTab.settings");
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

  const saveBar = (
    <>
      {hasPendingChanges ? <span className="dv-hint">{tSet("unsaved")}</span> : null}
      <Button size="sm" variant="ghost" onClick={() => void resetOverrides()} disabled={saving}>
        {tSet("reset")}
      </Button>
      <Button size="sm" variant={hasPendingChanges ? "primary" : "secondary"} onClick={() => void saveSettings()} disabled={!hasPendingChanges || saving}>
        {saving ? tSet("saving") : tSet("save")}
      </Button>
    </>
  );

  return (
    <>
      <section className="dv-sec" aria-labelledby="dst-agent-t">
        <div className="dv-sh">
          <h2 id="dst-agent-t">
            {t("title")}
            {payload ? (
              <span className={`dv-quiet${payload.connected ? "" : " bad"}`}>{payload.connected ? t("connected") : t("disconnected")}</span>
            ) : null}
          </h2>
          <button
            type="button"
            className="dst-icon"
            onClick={() => void refresh()}
            disabled={loading || saving || testingDocker}
            aria-label={t("refresh")}
            title={t("refresh")}
          >
            <RefreshCw aria-hidden="true" />
          </button>
        </div>
        {error ? <p className="dv-err dst-msg-line" role="alert">{error}</p> : null}
        {message ? <p className="dv-ok-text dst-msg-line" role="status">{message}</p> : null}
        {loading ? (
          <div className="dv-sheet"><p className="dv-hint">{t("loading")}</p></div>
        ) : !payload ? (
          <div className="dv-sheet"><p className="dv-hint">{t("none")}</p></div>
        ) : (
          <AgentSettingsOverviewSection
            payload={payload}
            formatTime={formatTime}
            testingDocker={testingDocker}
            onTestDocker={() => void testDocker()}
            dockerTestOutput={dockerTestOutput}
            forceAgentUpdate={forceAgentUpdate}
            onForceAgentUpdateChange={setForceAgentUpdate}
            updatingAgent={updatingAgent}
            onUpdateAgent={() => void updateAgent()}
            agentUpdateOutput={agentUpdateOutput}
          />
        )}
      </section>

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

      {!loading && payload ? (
        <AgentSettingsEditorSection
          serviceDiscoverySettings={serviceDiscoverySettings}
          generalSettings={generalSettings}
          draft={draft}
          settingKeyFilesRootMode={settingKeyFilesRootMode}
          onUpdateDraftValue={updateDraftValue}
          history={history}
          formatTime={formatTime}
          saveBar={saveBar}
        />
      ) : null}
    </>
  );
}
