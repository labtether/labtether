"use client";

import { useState, type Dispatch, type SetStateAction } from "react";
import { useTranslations } from "next-intl";
import type { RuntimeSettingEntry } from "../../../../console/models";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { runtimeScopeTitle, runtimeSettingWords } from "./runtimeSettingWords";
import { Section, Sheet, SheetFoot, Switch } from "./SettingsKit";

type AdvancedSettingsCardProps = {
  runtimeSettingsLoading: boolean;
  runtimeSettingsError: string | null;
  runtimeSettingsByScope: Array<[string, RuntimeSettingEntry[]]>;
  runtimeDraftValues: Record<string, string>;
  setRuntimeDraftValues: Dispatch<SetStateAction<Record<string, string>>>;
  runtimeSettingsSaving: boolean;
  runtimeSettingsMessage: string | null;
  saveRuntimeSettings: (keys?: string[]) => Promise<void> | void;
  resetRuntimeSetting: (key: string) => Promise<void> | void;
};

/**
 * Every hub runtime setting, grouped by scope, behind Show (the sections above hold the common ones). Each row: label,
 * what it does, its key; the value, where it comes from, Reset.
 */
export function AdvancedSettingsCard({
  runtimeSettingsLoading,
  runtimeSettingsError,
  runtimeSettingsByScope,
  runtimeDraftValues,
  setRuntimeDraftValues,
  runtimeSettingsSaving,
  runtimeSettingsMessage,
  saveRuntimeSettings,
  resetRuntimeSetting,
}: AdvancedSettingsCardProps) {
  const t = useTranslations("settings");
  const [open, setOpen] = useState(false);
  const setDraft = (key: string, value: string) => setRuntimeDraftValues((current) => ({ ...current, [key]: value }));
  const dirty = runtimeSettingsByScope.some(([, entries]) =>
    entries.some((entry) => (runtimeDraftValues[entry.key] ?? entry.effective_value).trim() !== entry.effective_value),
  );

  return (
    <Section
      title={t("advanced.title")}
      id="advanced"
      action={(
        <Button size="sm" variant="ghost" onClick={() => setOpen((value) => !value)} aria-expanded={open} aria-controls="st-advanced-list">
          {open ? t("advanced.hide") : t("advanced.show")}
        </Button>
      )}
    >
      {!open ? (
        runtimeSettingsError ? <Sheet><SheetFoot message={runtimeSettingsError} tone="bad" /></Sheet> : null
      ) : (
        <Sheet className={runtimeSettingsLoading && runtimeSettingsByScope.length === 0 ? "st-loading" : ""}>
          <div id="st-advanced-list">
            {runtimeSettingsByScope.map(([scope, entries]) => (
              <div key={scope} role="group" aria-labelledby={`st-scope-${scope}`}>
                <h3 className="st-sub st-pad-x" id={`st-scope-${scope}`}>{runtimeScopeTitle(t, scope)}</h3>
                {entries.map((entry) => (
                  <AdvancedRow
                    key={entry.key}
                    entry={entry}
                    value={runtimeDraftValues[entry.key] ?? entry.effective_value}
                    onChange={(value) => setDraft(entry.key, value)}
                    onReset={() => void resetRuntimeSetting(entry.key)}
                    disabled={runtimeSettingsSaving}
                  />
                ))}
              </div>
            ))}
          </div>
          <SheetFoot
            message={runtimeSettingsError ?? runtimeSettingsMessage}
            tone={runtimeSettingsError ? "bad" : undefined}
          >
            <Button size="sm" disabled={runtimeSettingsSaving || !dirty} onClick={() => void saveRuntimeSettings()}>
              {runtimeSettingsSaving ? t("advanced.saving") : t("advanced.save")}
            </Button>
          </SheetFoot>
        </Sheet>
      )}
    </Section>
  );
}

function AdvancedRow({
  entry,
  value,
  onChange,
  onReset,
  disabled,
}: {
  entry: RuntimeSettingEntry;
  value: string;
  onChange: (value: string) => void;
  onReset: () => void;
  disabled: boolean;
}) {
  const t = useTranslations("settings");
  const id = `st-rt-${entry.key.replace(/[^a-z0-9]+/gi, "-")}`;
  const { label, description } = runtimeSettingWords(t, entry);
  return (
    <div className="st-row two" data-setting-key={entry.key}>
      <div className="st-row-l">
        {entry.type === "bool" ? <span className="st-nm">{label}</span> : <label className="st-nm" htmlFor={id}>{label}</label>}
        {description ? <span className="st-g">{description}</span> : null}
        <code className="st-mono">{entry.key}</code>
      </div>
      <div className="st-row-r">
        <span className="st-g">{t(`advanced.source.${entry.source}`)}</span>
        {entry.source === "ui" ? (
          <Button variant="ghost" size="sm" onClick={onReset} disabled={disabled} aria-label={t("advanced.resetKey", { key: label })}>
            {t("advanced.reset")}
          </Button>
        ) : null}
        <RuntimeControl id={id} entry={{ ...entry, label }} value={value} onChange={onChange} />
      </div>
    </div>
  );
}

/** The input that fits a runtime setting's type. A sensitive value is never shown; empty keeps it. */
export function RuntimeControl({
  id,
  entry,
  value,
  onChange,
  testId,
}: {
  id: string;
  entry: RuntimeSettingEntry;
  value: string;
  onChange: (value: string) => void;
  testId?: string;
}) {
  const t = useTranslations("settings");
  if (entry.type === "bool") {
    return (
      <span data-testid={testId}>
        <Switch checked={value === "true"} onChange={(next) => onChange(String(next))} label={entry.label} />
      </span>
    );
  }
  if (entry.type === "enum") {
    return (
      <Select id={id} className="st-in-s" value={value} onChange={(event) => onChange(event.target.value)} data-testid={testId}>
        {(entry.allowed_values ?? []).map((option) => (
          <option key={option} value={option}>{option}</option>
        ))}
      </Select>
    );
  }
  if (entry.type === "int") {
    return (
      <Input
        id={id}
        className="st-in-xs st-num"
        type="number"
        step={1}
        min={entry.min_int}
        max={entry.max_int}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        data-testid={testId}
      />
    );
  }
  return (
    <Input
      id={id}
      className="st-in-m"
      type={entry.sensitive ? "password" : undefined}
      value={value}
      placeholder={entry.sensitive && entry.configured ? t("advanced.keepSecret") : undefined}
      autoComplete={entry.sensitive ? "new-password" : undefined}
      onChange={(event) => onChange(event.target.value)}
      data-testid={testId}
    />
  );
}
