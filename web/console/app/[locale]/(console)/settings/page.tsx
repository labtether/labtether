"use client";

import { useCallback, useMemo, useState, useEffect } from "react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { useTheme, accentOptions } from "../../../contexts/ThemeContext";
import { useStatusAssetNameMap } from "../../../contexts/StatusContext";
import { useSettingsForm } from "../../../hooks/useSettingsForm";
import { useWebServices } from "../../../hooks/useWebServices";
import {
  densityOptions,
  runtimeSettingKeys,
  themeOptions,
} from "../../../console/models";
import { Card } from "../../../components/ui/Card";
import { SegmentedTabs } from "../../../components/ui/SegmentedTabs";
import { AdvancedSettingsCard } from "./components/AdvancedSettingsCard";
import { RetentionSettingsCard } from "./components/RetentionSettingsCard";
import { ServiceDiscoveryDefaultsCard } from "./components/ServiceDiscoveryDefaultsCard";
import { ServicesDiscoveryOverviewCard } from "../services/ServicesDiscoveryOverviewCard";
import { buildDiscoveryOverview } from "../services/servicesDiscoveryHelpers";
import { AboutCard } from "./components/AboutCard";
import { NotificationChannelsCard } from "./components/NotificationChannelsCard";
import { PrometheusExportCard } from "./components/PrometheusExportCard";
import { BackupExportCard } from "./components/BackupExportCard";
import { ApiKeysCard } from "./components/ApiKeysCard";
import { McpConnectionCard } from "./components/McpConnectionCard";
import { ApiReferenceCard } from "./components/ApiReferenceCard";

const SECTION_HEADING = "text-xs font-semibold uppercase tracking-wider text-[var(--muted)]";

type SettingsTab = "general" | "apiAccess" | "advanced";
const TAB_OPTIONS: SettingsTab[] = ["general", "apiAccess", "advanced"];
const TAB_LS_KEY = "labtether-settings-tab";

const POLL_INTERVAL_KEY = runtimeSettingKeys.pollIntervalSeconds;
const POLL_INTERVAL_VALUES = ["0", "5", "10", "30", "60"] as const;
type PollIntervalValue = (typeof POLL_INTERVAL_VALUES)[number];

export default function SettingsPage() {
  const t = useTranslations('settings');
  const [activeTab, setActiveTab] = useState<SettingsTab>(() => {
    if (typeof window === "undefined") return "general";
    const stored = localStorage.getItem(TAB_LS_KEY) as SettingsTab | null;
    return stored && TAB_OPTIONS.includes(stored) ? stored : "general";
  });

  const handleTabChange = useCallback((tab: SettingsTab) => {
    setActiveTab(tab);
    localStorage.setItem(TAB_LS_KEY, tab);
  }, []);

  const { theme, setTheme, density, setDensity, accent, setAccent } = useTheme();
  const assetNameMap = useStatusAssetNameMap();
  const { discoveryStats } = useWebServices({});
  const discoveryOverview = useMemo(
    () => buildDiscoveryOverview(discoveryStats, assetNameMap),
    [discoveryStats, assetNameMap],
  );
  const {
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
    retentionPresets,
    retentionDraftValues,
    setRetentionDraftValues,
    retentionLoading,
    retentionSaving,
    retentionMessage,
    applyRetentionPreset,
    saveRetentionSettings
  } = useSettingsForm();

  const [pendingPollSave, setPendingPollSave] = useState(false);

  useEffect(() => {
    if (pendingPollSave) {
      setPendingPollSave(false);
      void saveRuntimeSettings([POLL_INTERVAL_KEY]);
    }
  }, [pendingPollSave, saveRuntimeSettings]);

  const handlePollIntervalChange = useCallback((value: PollIntervalValue) => {
    setRuntimeDraftValues((prev) => ({ ...prev, [POLL_INTERVAL_KEY]: value }));
    setPendingPollSave(true);
  }, [setRuntimeDraftValues]);

  const pollIntervalValue: PollIntervalValue = POLL_INTERVAL_VALUES.includes(
    runtimeDraftValues[POLL_INTERVAL_KEY] as PollIntervalValue
  )
    ? (runtimeDraftValues[POLL_INTERVAL_KEY] as PollIntervalValue)
    : "5";

  return (
    <>
      <PageHeader title={t('title')} subtitle={t('subtitle')} />

      <div className="mb-6">
        <SegmentedTabs
          value={activeTab}
          options={TAB_OPTIONS.map((tab) => ({ id: tab, label: t(`tabs.${tab}`) }))}
          onChange={handleTabChange}
        />
      </div>

      {activeTab === "general" && (
        <>
          {/* Appearance */}
          <Card className="mb-6">
            <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-3">{t('appearance.heading')}</p>
            <div className="flex flex-wrap items-center gap-6">
              <div className="flex items-center gap-2">
                <span className="text-xs text-[var(--muted)]">{t('appearance.theme')}</span>
                <SegmentedTabs
                  value={theme}
                  options={themeOptions.map((option) => ({ id: option.id, label: option.label }))}
                  onChange={setTheme}
                />
              </div>
              <div className="flex items-center gap-2">
                <span className="text-xs text-[var(--muted)]">{t('appearance.layout')}</span>
                <SegmentedTabs
                  value={density}
                  options={densityOptions.map((option) => ({ id: option.id, label: option.label }))}
                  onChange={setDensity}
                />
              </div>
              <div className="flex items-center gap-2">
                <span className="text-xs text-[var(--muted)]">{t('appearance.accent')}</span>
                <SegmentedTabs
                  value={accent}
                  options={accentOptions.map((option) => ({ id: option.id, label: option.label }))}
                  onChange={setAccent}
                />
              </div>
              <div className="flex items-center gap-2">
                <span className="text-xs text-[var(--muted)]">{t('appearance.autoRefreshLabel')}</span>
                <SegmentedTabs
                  value={pollIntervalValue}
                  options={[
                    { id: "0", label: t('appearance.autoRefreshOff') },
                    { id: "5", label: "5s" },
                    { id: "10", label: "10s" },
                    { id: "30", label: "30s" },
                    { id: "60", label: "60s" },
                  ]}
                  onChange={handlePollIntervalChange}
                />
              </div>
            </div>
          </Card>

          <NotificationChannelsCard />

          <PrometheusExportCard />

          <BackupExportCard />

          <AboutCard />
        </>
      )}

      {activeTab === "apiAccess" && (
        <>
          <ApiKeysCard />

          <McpConnectionCard />

          <ApiReferenceCard />
        </>
      )}

      {activeTab === "advanced" && (
        <>
          <ServiceDiscoveryDefaultsCard
            runtimeSettings={runtimeSettings}
            runtimeDraftValues={runtimeDraftValues}
            setRuntimeDraftValues={setRuntimeDraftValues}
            runtimeSettingsLoading={runtimeSettingsLoading}
            runtimeSettingsSaving={runtimeSettingsSaving}
            saveRuntimeSettings={saveRuntimeSettings}
          />

          <ServicesDiscoveryOverviewCard discoveryOverview={discoveryOverview} />

          <AdvancedSettingsCard
            sectionHeadingClassName={SECTION_HEADING}
            runtimeSettingsLoading={runtimeSettingsLoading}
            runtimeSettingsError={runtimeSettingsError}
            runtimeSettingsByScope={runtimeSettingsByScope}
            runtimeDraftValues={runtimeDraftValues}
            setRuntimeDraftValues={setRuntimeDraftValues}
            runtimeSettingsSaving={runtimeSettingsSaving}
            runtimeSettingsMessage={runtimeSettingsMessage}
            saveRuntimeSettings={saveRuntimeSettings}
            resetRuntimeSetting={resetRuntimeSetting}
          />

          <RetentionSettingsCard
            sectionHeadingClassName={SECTION_HEADING}
            retentionPresets={retentionPresets}
            retentionDraftValues={retentionDraftValues}
            setRetentionDraftValues={setRetentionDraftValues}
            retentionLoading={retentionLoading}
            retentionSaving={retentionSaving}
            retentionMessage={retentionMessage}
            applyRetentionPreset={applyRetentionPreset}
            saveRetentionSettings={saveRetentionSettings}
          />
        </>
      )}
    </>
  );
}
