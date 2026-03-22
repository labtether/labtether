"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { useAuth } from "../../../contexts/AuthContext";
import { useToast } from "../../../contexts/ToastContext";
import { useSettingsForm } from "../../../hooks/useSettingsForm";
import { useEnrollment } from "../../../hooks/useEnrollment";
import { ConnectAgentsCard } from "../settings/components/ConnectAgentsCard";
import { DangerZoneCard } from "../settings/components/DangerZoneCard";
import { ManagedDatabaseCard } from "../settings/components/ManagedDatabaseCard";
import { OIDCSettingsCard } from "../settings/components/OIDCSettingsCard";
import { TailscaleServeCard } from "../settings/components/TailscaleServeCard";
import { TLSManagementCard } from "../settings/components/TLSManagementCard";

const SECTION_HEADING = "text-xs font-semibold uppercase tracking-wider text-[var(--muted)]";

export default function SecurityPage() {
  const t = useTranslations('security');
  const { user } = useAuth();
  const { addToast } = useToast();
  const {
    runtimeSettings,
    runtimeDraftValues,
    setRuntimeDraftValues,
    runtimeSettingsLoading,
    runtimeSettingsSaving,
    runtimeSettingsMessage,
    saveRuntimeSettings,
    resetRuntimeSetting,
  } = useSettingsForm();

  const {
    enrollmentTokens, agentTokens, hubCandidates, hubURL, wsURL, selectHubURL,
    loading: enrollLoading, error: enrollError,
    newRawToken, generating,
    generateToken, revokeEnrollmentToken, revokeAgentToken, cleanupDeadTokens, clearNewToken,
  } = useEnrollment();

  const [tokenLabel, setTokenLabel] = useState("");
  const [tokenTTL, setTokenTTL] = useState(24);
  const [tokenMaxUses, setTokenMaxUses] = useState(1);
  const [copied, setCopied] = useState("");
  const canManageUsers = user?.role === "owner" || user?.role === "admin";

  const copyToClipboard = (text: string, label: string, toastMessage?: string) => {
    void navigator.clipboard.writeText(text);
    setCopied(label);
    setTimeout(() => setCopied(""), 2000);
    addToast("success", toastMessage ?? "Copied to clipboard");
  };

  if (!canManageUsers) {
    return (
      <>
        <PageHeader title={t('title')} subtitle={t('subtitle')} />
        <p className="text-sm text-[var(--muted)]">Only admin/owner users can manage security settings.</p>
      </>
    );
  }

  return (
    <>
      <PageHeader title={t('title')} subtitle={t('subtitle')} />

      <TLSManagementCard copyToClipboard={copyToClipboard} copied={copied} />

      <TailscaleServeCard
        copyToClipboard={copyToClipboard}
        copied={copied}
        runtimeSettings={runtimeSettings}
        runtimeDraftValues={runtimeDraftValues}
        setRuntimeDraftValues={setRuntimeDraftValues}
        runtimeSettingsLoading={runtimeSettingsLoading}
        runtimeSettingsSaving={runtimeSettingsSaving}
        runtimeSettingsMessage={runtimeSettingsMessage}
        saveRuntimeSettings={saveRuntimeSettings}
        resetRuntimeSetting={resetRuntimeSetting}
        canManageActions={canManageUsers}
      />

      <OIDCSettingsCard canManageUsers={canManageUsers} />

      <ConnectAgentsCard
        sectionHeadingClassName={SECTION_HEADING}
        enrollmentTokens={enrollmentTokens}
        agentTokens={agentTokens}
        hubCandidates={hubCandidates}
        hubURL={hubURL}
        wsURL={wsURL}
        selectHubURL={selectHubURL}
        enrollLoading={enrollLoading}
        enrollError={enrollError}
        newRawToken={newRawToken}
        generating={generating}
        generateToken={generateToken}
        revokeEnrollmentToken={revokeEnrollmentToken}
        revokeAgentToken={revokeAgentToken}
        cleanupDeadTokens={cleanupDeadTokens}
        clearNewToken={clearNewToken}
        tokenLabel={tokenLabel}
        setTokenLabel={setTokenLabel}
        tokenTTL={tokenTTL}
        setTokenTTL={setTokenTTL}
        tokenMaxUses={tokenMaxUses}
        setTokenMaxUses={setTokenMaxUses}
        copyToClipboard={copyToClipboard}
        copied={copied}
      />

      {canManageUsers ? (
        <ManagedDatabaseCard copyToClipboard={copyToClipboard} copied={copied} />
      ) : null}

      <div className="border-t border-[var(--line)] my-2" />
      <DangerZoneCard />
    </>
  );
}
