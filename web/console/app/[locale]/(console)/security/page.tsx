"use client";

import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { useAuth } from "../../../contexts/AuthContext";
import { useSettingsForm } from "../../../hooks/useSettingsForm";
import { isOwnerRole } from "../../../lib/roles";
import { ApiKeysCard } from "../settings/components/ApiKeysCard";
import { ApiReferenceCard } from "../settings/components/ApiReferenceCard";
import { CredentialProfilesCard } from "../settings/components/CredentialProfilesCard";
import { McpConnectionCard } from "../settings/components/McpConnectionCard";
import { OIDCSettingsCard } from "../settings/components/OIDCSettingsCard";
import { PrometheusExportCard } from "../settings/components/PrometheusExportCard";
import { Section, Sheet } from "../settings/components/SettingsKit";
import { TailscaleServeCard } from "../settings/components/TailscaleServeCard";
import { TLSManagementCard } from "../settings/components/TLSManagementCard";

/** Settings › Security: HTTPS (certificate, Tailscale Serve), single sign-on, saved logins, API keys, Prometheus, MCP. */
export default function SecurityPage() {
  const t = useTranslations("security");
  const { user } = useAuth();
  const canManage = user?.role === "owner" || user?.role === "admin";

  if (!canManage) {
    return (
      <>
        <PageHeader title={t("title")} />
        <Sheet pad>
          <p className="st-msg">{t("adminOnly")}</p>
        </Sheet>
      </>
    );
  }

  return <SecuritySettings canEditSSO={isOwnerRole(user?.role)} />;
}

function SecuritySettings({ canEditSSO }: { canEditSSO: boolean }) {
  const t = useTranslations("security");
  const form = useSettingsForm();

  return (
    <>
      <PageHeader title={t("title")} />
      <div className="st">
        <Section title={t("https")} id="https">
          <Sheet>
            <TLSManagementCard />
            <TailscaleServeCard
              runtimeSettings={form.runtimeSettings}
              runtimeDraftValues={form.runtimeDraftValues}
              setRuntimeDraftValues={form.setRuntimeDraftValues}
              runtimeSettingsLoading={form.runtimeSettingsLoading}
              runtimeSettingsSaving={form.runtimeSettingsSaving}
              runtimeSettingsMessage={form.runtimeSettingsMessage}
              saveRuntimeSettings={form.saveRuntimeSettings}
              resetRuntimeSetting={form.resetRuntimeSetting}
              canManageActions
            />
          </Sheet>
        </Section>
        <OIDCSettingsCard canManageUsers canEdit={canEditSSO} />
        <CredentialProfilesCard />
        <ApiKeysCard />
        <PrometheusExportCard />
        <McpConnectionCard />
        <ApiReferenceCard />
      </div>
    </>
  );
}
