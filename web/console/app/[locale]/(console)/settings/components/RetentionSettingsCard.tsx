"use client";

import type { Dispatch, SetStateAction } from "react";
import { useTranslations } from "next-intl";
import { retentionFieldDefs, type RetentionSettingsPayload } from "../../../../console/models";
import { Button } from "../../../../components/ui/Button";
import { Input } from "../../../../components/ui/Input";
import { Row, Section, Sheet, SheetFoot } from "./SettingsKit";

type RetentionSettingsCardProps = {
  retentionSettings: RetentionSettingsPayload["settings"] | null;
  retentionPresets: RetentionSettingsPayload["presets"];
  retentionDraftValues: Record<string, string>;
  setRetentionDraftValues: Dispatch<SetStateAction<Record<string, string>>>;
  retentionLoading: boolean;
  retentionSaving: boolean;
  retentionMessage: string | null;
  applyRetentionPreset: (presetID: string) => Promise<void> | void;
  saveRetentionSettings: () => Promise<void> | void;
};

/** How long the hub keeps each kind of history. Presets apply at once; edited windows wait for Save. */
export function RetentionSettingsCard({
  retentionSettings,
  retentionPresets,
  retentionDraftValues,
  setRetentionDraftValues,
  retentionLoading,
  retentionSaving,
  retentionMessage,
  applyRetentionPreset,
  saveRetentionSettings,
}: RetentionSettingsCardProps) {
  const t = useTranslations("settings");
  const dirty = retentionFieldDefs.some(
    (field) => (retentionDraftValues[field.key] ?? "") !== (retentionSettings?.[field.key] ?? ""),
  );

  return (
    <Section title={t("retention.title")} id="retention">
      <Sheet className={retentionLoading && !retentionSettings ? "st-loading" : ""}>
        {retentionFieldDefs.map((field) => {
          const id = `st-retention-${field.key}`;
          return (
            <Row key={field.key} label={<label htmlFor={id}>{t(`retention.fields.${field.key}`)}</label>}>
              <Input
                id={id}
                className="st-in-s"
                value={retentionDraftValues[field.key] ?? ""}
                onChange={(event) => setRetentionDraftValues((current) => ({ ...current, [field.key]: event.target.value }))}
                placeholder={t("retention.placeholder")}
                disabled={retentionLoading}
              />
            </Row>
          );
        })}
        <SheetFoot message={retentionMessage}>
          {retentionPresets.length > 0 ? <span className="st-label">{t("retention.presets")}</span> : null}
          {retentionPresets.map((preset) => (
            <Button
              key={preset.id}
              size="sm"
              variant="ghost"
              title={preset.description || undefined}
              disabled={retentionSaving}
              onClick={() => void applyRetentionPreset(preset.id)}
            >
              {preset.name}
            </Button>
          ))}
          <Button size="sm" disabled={retentionSaving || !dirty} loading={retentionSaving} onClick={() => void saveRetentionSettings()}>
            {t("retention.save")}
          </Button>
        </SheetFoot>
      </Sheet>
    </Section>
  );
}
