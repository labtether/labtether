"use client";

import { useCallback, useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { useTranslations } from "next-intl";
import { localeLabels, supportedLocales, useSwitchLocale } from "../../../../components/LanguagePicker";
import { Select } from "../../../../components/ui/Input";
import { SegmentedTabs } from "../../../../components/ui/SegmentedTabs";
import { accentOptions, useTheme } from "../../../../contexts/ThemeContext";
import { densityOptions, runtimeSettingKeys, themeOptions } from "../../../../console/models";
import { Row, Section, Sheet } from "./SettingsKit";

const POLL_INTERVAL_KEY = runtimeSettingKeys.pollIntervalSeconds;
const POLL_INTERVAL_VALUES = ["0", "5", "10", "30", "60"] as const;
type PollIntervalValue = (typeof POLL_INTERVAL_VALUES)[number];

type AppearanceSectionProps = {
  runtimeDraftValues: Record<string, string>;
  setRuntimeDraftValues: Dispatch<SetStateAction<Record<string, string>>>;
  saveRuntimeSettings: (keys?: string[]) => Promise<void> | void;
};

/** Theme, layout, accent and language are this browser's; the refresh interval is the hub's poll setting. */
export function AppearanceSection({ runtimeDraftValues, setRuntimeDraftValues, saveRuntimeSettings }: AppearanceSectionProps) {
  const t = useTranslations("settings");
  const { theme, setTheme, density, setDensity, accent, setAccent } = useTheme();
  const { locale, switchTo } = useSwitchLocale();
  const [pendingPollSave, setPendingPollSave] = useState(false);

  // The draft must be in state before the save reads it, so the save runs one render later.
  useEffect(() => {
    if (!pendingPollSave) return;
    setPendingPollSave(false);
    void saveRuntimeSettings([POLL_INTERVAL_KEY]);
  }, [pendingPollSave, saveRuntimeSettings]);

  const handlePollIntervalChange = useCallback((value: PollIntervalValue) => {
    setRuntimeDraftValues((prev) => ({ ...prev, [POLL_INTERVAL_KEY]: value }));
    setPendingPollSave(true);
  }, [setRuntimeDraftValues]);

  const pollIntervalValue: PollIntervalValue = POLL_INTERVAL_VALUES.includes(runtimeDraftValues[POLL_INTERVAL_KEY] as PollIntervalValue)
    ? (runtimeDraftValues[POLL_INTERVAL_KEY] as PollIntervalValue)
    : "5";

  return (
    <Section title={t("appearance.heading")} id="appearance">
      <Sheet>
        <Row label={t("appearance.theme")} className="st-seg-row">
          <SegmentedTabs
            value={theme}
            options={themeOptions.map((option) => ({ id: option.id, label: option.id === "oled" ? option.label : t(`appearance.themes.${option.id}`) }))}
            onChange={setTheme}
          />
        </Row>
        <Row label={t("appearance.layout")} className="st-seg-row">
          <SegmentedTabs
            value={density}
            options={densityOptions.map((option) => ({ id: option.id, label: t(`appearance.layouts.${option.id}`) }))}
            onChange={setDensity}
          />
        </Row>
        <Row label={t("appearance.accent")} className="st-seg-row">
          <SegmentedTabs
            value={accent}
            options={accentOptions.map((option) => ({ id: option.id, label: option.label }))}
            onChange={setAccent}
          />
        </Row>
        <Row label={<label htmlFor="st-language">{t("appearance.language")}</label>}>
          <Select id="st-language" value={locale} onChange={(event) => switchTo(event.target.value)}>
            {supportedLocales.map((loc) => (
              <option key={loc} value={loc}>{localeLabels[loc]}</option>
            ))}
          </Select>
        </Row>
        <Row label={t("appearance.autoRefreshLabel")} className="st-seg-row">
          <SegmentedTabs
            value={pollIntervalValue}
            options={[
              { id: "0", label: t("appearance.autoRefreshOff") },
              { id: "5", label: "5s" },
              { id: "10", label: "10s" },
              { id: "30", label: "30s" },
              { id: "60", label: "60s" },
            ]}
            onChange={handlePollIntervalChange}
          />
        </Row>
      </Sheet>
    </Section>
  );
}
