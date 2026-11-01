"use client";

import { useEffect } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { useRouter } from "../../../../i18n/navigation";
import { PageHeader } from "../../../components/PageHeader";
import { useAuth } from "../../../contexts/AuthContext";
import { useSettingsForm } from "../../../hooks/useSettingsForm";
import { hasAdminRole } from "../../../lib/roles";
import { AboutCard } from "./components/AboutCard";
import { AdvancedSettingsCard } from "./components/AdvancedSettingsCard";
import { AppearanceSection } from "./components/AppearanceSection";
import { BackupExportCard } from "./components/BackupExportCard";
import { DangerZoneCard } from "./components/DangerZoneCard";
import { ManagedDatabaseCard } from "./components/ManagedDatabaseCard";
import { RetentionSettingsCard } from "./components/RetentionSettingsCard";
import { ServiceDiscoveryDefaultsCard } from "./components/ServiceDiscoveryDefaultsCard";
import { Sheet } from "./components/SettingsKit";

/** Old ?tab= links (the ⌘K palette, bookmarks): the section of General that holds them now, or the page that does. */
const OLD_TABS: Record<string, { anchor?: string; href?: string }> = {
  general: { anchor: "appearance" },
  advanced: { anchor: "discovery" },
  apiAccess: { href: "/security#api-tokens" },
};

export default function SettingsPage() {
  const t = useTranslations("settings");
  const { user } = useAuth();

  if (!hasAdminRole(user?.role)) {
    return (
      <>
        <PageHeader title={t("title")} />
        <Sheet pad>
          <p className="st-msg">{t("general.adminOnly")}</p>
        </Sheet>
      </>
    );
  }

  return <GeneralSettings />;
}

function GeneralSettings() {
  const t = useTranslations("settings");
  const router = useRouter();
  const requestedTab = useSearchParams().get("tab");
  const form = useSettingsForm();

  useEffect(() => {
    const target = requestedTab ? OLD_TABS[requestedTab] : undefined;
    if (!target) return;
    if (target.href) {
      router.replace(target.href);
      return;
    }
    document.getElementById(target.anchor ?? "")?.scrollIntoView({ block: "start" });
  }, [requestedTab, router]);

  return (
    <>
      <PageHeader title={t("title")} />
      <div className="st">
        <AppearanceSection
          runtimeDraftValues={form.runtimeDraftValues}
          setRuntimeDraftValues={form.setRuntimeDraftValues}
          saveRuntimeSettings={form.saveRuntimeSettings}
        />

        <RetentionSettingsCard
          retentionSettings={form.retentionSettings}
          retentionPresets={form.retentionPresets}
          retentionDraftValues={form.retentionDraftValues}
          setRetentionDraftValues={form.setRetentionDraftValues}
          retentionLoading={form.retentionLoading}
          retentionSaving={form.retentionSaving}
          retentionMessage={form.retentionMessage}
          applyRetentionPreset={form.applyRetentionPreset}
          saveRetentionSettings={form.saveRetentionSettings}
        />

        <AdvancedSettingsCard
          runtimeSettingsLoading={form.runtimeSettingsLoading}
          runtimeSettingsError={form.runtimeSettingsError}
          runtimeSettingsByScope={form.runtimeSettingsByScope}
          runtimeDraftValues={form.runtimeDraftValues}
          setRuntimeDraftValues={form.setRuntimeDraftValues}
          runtimeSettingsSaving={form.runtimeSettingsSaving}
          runtimeSettingsMessage={form.runtimeSettingsMessage}
          saveRuntimeSettings={form.saveRuntimeSettings}
          resetRuntimeSetting={form.resetRuntimeSetting}
        />

        <ServiceDiscoveryDefaultsCard
          runtimeSettings={form.runtimeSettings}
          runtimeDraftValues={form.runtimeDraftValues}
          setRuntimeDraftValues={form.setRuntimeDraftValues}
          runtimeSettingsLoading={form.runtimeSettingsLoading}
          runtimeSettingsSaving={form.runtimeSettingsSaving}
          runtimeSettingsMessage={form.runtimeSettingsMessage}
          saveRuntimeSettings={form.saveRuntimeSettings}
        />

        <ManagedDatabaseCard />
        <BackupExportCard />
        <AboutCard />
        <DangerZoneCard />
      </div>
    </>
  );
}
